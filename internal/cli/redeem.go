package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type redeemAPIResponse struct {
	Code         string     `json:"code"`
	CreditAmount int        `json:"credit_amount"`
	NewBalance   int        `json:"new_balance"`
	RedeemedAt   time.Time  `json:"redeemed_at"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}

type redeemAPIErrorEnvelope struct {
	Data struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	} `json:"data"`
	Error string `json:"error"`
}

// RedeemHelpText is shown by `officecli help redeem` and on missing args.
func RedeemHelpText() string {
	return strings.TrimSpace(`Usage:
  officecli redeem <code>
  officecli redeem --code <code>

Redeem an OfficeCLI redemption code to add hosted credits to your account.

Options:
  --code <value>   Redemption code (alternative to positional arg)
  --json           Output the response as JSON instead of human text
  --source <name>  Client source tag (cli|tui|desktop); defaults to cli.

Examples:
  officecli redeem PROMO2026
  officecli redeem --code PROMO2026 --json

You must be logged in first via "officecli login".`) + "\n"
}

// runRedeem is the entry point for `officecli redeem ...`. It validates the
// session, calls /api/cli/redemption-codes/redeem, and renders a friendly
// summary (or JSON when --json is passed).
func (a *App) runRedeem(ctx context.Context, cfg Config, args []string) error {
	code, jsonOut, source, err := parseRedeemArgs(args)
	if err != nil {
		_, _ = io.WriteString(a.Stderr, err.Error()+"\n\n"+RedeemHelpText())
		return err
	}
	if strings.TrimSpace(cfg.License.SessionToken) == "" {
		msg := "you must log in first. Run: officecli login"
		_, _ = io.WriteString(a.Stderr, msg+"\n")
		return errors.New(msg)
	}
	if strings.TrimSpace(cfg.License.BaseURL) == "" {
		return errors.New("license base URL is not configured; run: officecli config init")
	}
	if source == "" {
		source = "cli"
	}
	payload := map[string]string{"code": code, "source": source}
	var resp redeemAPIResponse
	err = a.platformJSON(ctx, cfg.License.BaseURL, http.MethodPost, "/api/cli/redemption-codes/redeem", payload, cfg.License.SessionToken, &resp)
	if err != nil {
		return renderRedeemError(a.Stderr, err)
	}
	if jsonOut {
		enc := json.NewEncoder(a.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(resp)
	}
	if _, err := fmt.Fprintf(a.Stdout, "Redeemed %s: +%d credits. New balance: %d\n", resp.Code, resp.CreditAmount, resp.NewBalance); err != nil {
		return err
	}
	if resp.ExpiresAt != nil && !resp.ExpiresAt.IsZero() {
		_, _ = fmt.Fprintf(a.Stdout, "Code expires at: %s\n", resp.ExpiresAt.Format(time.RFC3339))
	}
	return nil
}

func parseRedeemArgs(args []string) (string, bool, string, error) {
	var code, source string
	var jsonOut bool
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			jsonOut = true
		case arg == "--help" || arg == "-h":
			return "", false, "", errors.New("help requested")
		case arg == "--code":
			if i+1 >= len(args) {
				return "", false, "", errors.New("--code requires a value")
			}
			i++
			code = args[i]
		case strings.HasPrefix(arg, "--code="):
			code = strings.TrimPrefix(arg, "--code=")
		case arg == "--source":
			if i+1 >= len(args) {
				return "", false, "", errors.New("--source requires a value")
			}
			i++
			source = args[i]
		case strings.HasPrefix(arg, "--source="):
			source = strings.TrimPrefix(arg, "--source=")
		case strings.HasPrefix(arg, "-"):
			return "", false, "", fmt.Errorf("unknown flag: %s", arg)
		default:
			if code == "" {
				code = arg
			} else {
				return "", false, "", fmt.Errorf("unexpected argument: %s", arg)
			}
		}
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return "", false, "", errors.New("redemption code is required")
	}
	source = strings.TrimSpace(strings.ToLower(source))
	return code, jsonOut, source, nil
}

// renderRedeemError translates platform error envelopes into actionable user
// messages and returns the original error so callers can propagate status.
func renderRedeemError(w io.Writer, err error) error {
	msg := err.Error()
	if idx := strings.Index(msg, "{"); idx >= 0 {
		var envelope redeemAPIErrorEnvelope
		if jsonErr := json.Unmarshal([]byte(msg[idx:]), &envelope); jsonErr == nil {
			if envelope.Data.Error.Code != "" {
				_, _ = fmt.Fprintf(w, "Redeem failed: %s\n", friendlyRedeemError(envelope.Data.Error.Code))
				return err
			}
			if envelope.Error != "" {
				_, _ = fmt.Fprintf(w, "Redeem failed: %s\n", envelope.Error)
				return err
			}
		}
	}
	_, _ = fmt.Fprintf(w, "Redeem failed: %s\n", msg)
	return err
}

func friendlyRedeemError(code string) string {
	switch code {
	case "code_not_found":
		return "redemption code not found"
	case "code_disabled":
		return "this redemption code is currently disabled"
	case "code_expired":
		return "this redemption code has expired"
	case "code_exhausted":
		return "this redemption code has been fully redeemed"
	case "code_already_claimed":
		return "you have already redeemed this code"
	case "code_required":
		return "redemption code is required"
	default:
		return code
	}
}
