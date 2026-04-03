package license

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type State struct {
	FingerprintHash string    `json:"fingerprint_hash"`
	LastCheckAt     time.Time `json:"last_check_at,omitempty"`
	LastDecision    string    `json:"last_decision,omitempty"`
	LastError       string    `json:"last_error,omitempty"`
	LastMode        string    `json:"last_mode,omitempty"`
}

func StatePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "officecli", "license-state.json")
}

func LoadState() (State, error) {
	path := StatePath()
	if path == "" {
		return State{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, nil
		}
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, err
	}
	return state, nil
}

func SaveState(state State) error {
	path := StatePath()
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
