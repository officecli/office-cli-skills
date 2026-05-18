package clisession

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/officecli/officecli-internal/platform/internal/model"
)

type fakeStore struct {
	challenges map[string]*model.CLILoginChallenge
	users      map[uint64]*model.User
	sessions   []*model.CLISession
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		challenges: map[string]*model.CLILoginChallenge{},
		users:      map[uint64]*model.User{},
	}
}

func (f *fakeStore) CreateCLILoginChallenge(_ context.Context, challenge *model.CLILoginChallenge) error {
	copied := *challenge
	f.challenges[challenge.ChallengeID] = &copied
	return nil
}

func (f *fakeStore) GetCLILoginChallengeByChallengeID(_ context.Context, challengeID string) (*model.CLILoginChallenge, error) {
	challenge := f.challenges[challengeID]
	if challenge == nil {
		return nil, nil
	}
	copied := *challenge
	return &copied, nil
}

func (f *fakeStore) GetCLILoginChallengeByUserCodeHash(_ context.Context, userCodeHash string) (*model.CLILoginChallenge, error) {
	for _, challenge := range f.challenges {
		if challenge.UserCodeHash != nil && *challenge.UserCodeHash == userCodeHash {
			copied := *challenge
			return &copied, nil
		}
	}
	return nil, nil
}

func (f *fakeStore) CompleteCLILoginChallenge(_ context.Context, challengeID string, userID uint64, exchangeCodeHash string, completedAt time.Time) (*model.CLILoginChallenge, error) {
	challenge := f.challenges[challengeID]
	if challenge == nil {
		return nil, nil
	}
	challenge.UserID = &userID
	challenge.ExchangeCodeHash = &exchangeCodeHash
	challenge.CompletedAt = &completedAt
	challenge.Status = model.CLILoginChallengeStatusCompleted
	copied := *challenge
	return &copied, nil
}

func (f *fakeStore) CompleteCLILoginChallengeByUserCodeHash(_ context.Context, userCodeHash string, userID uint64, completedAt time.Time) (*model.CLILoginChallenge, error) {
	for _, challenge := range f.challenges {
		if challenge.UserCodeHash != nil && *challenge.UserCodeHash == userCodeHash {
			challenge.UserID = &userID
			challenge.CompletedAt = &completedAt
			challenge.Status = model.CLILoginChallengeStatusCompleted
			copied := *challenge
			return &copied, nil
		}
	}
	return nil, nil
}

func (f *fakeStore) ConsumeCLILoginChallenge(_ context.Context, challengeID string, consumedAt time.Time) error {
	challenge := f.challenges[challengeID]
	if challenge == nil {
		return nil
	}
	challenge.ConsumedAt = &consumedAt
	challenge.Status = model.CLILoginChallengeStatusConsumed
	return nil
}

func (f *fakeStore) CreateCLISession(_ context.Context, session *model.CLISession) error {
	copied := *session
	copied.ID = uint64(len(f.sessions) + 1)
	f.sessions = append(f.sessions, &copied)
	return nil
}

func (f *fakeStore) FindCLISessionByTokenHash(_ context.Context, tokenHash string) (*model.CLISession, error) {
	for _, session := range f.sessions {
		if session.TokenHash == tokenHash {
			copied := *session
			return &copied, nil
		}
	}
	return nil, nil
}

func (f *fakeStore) TouchCLISession(_ context.Context, _ uint64, _ time.Time) error { return nil }

func (f *fakeStore) RevokeCLISession(_ context.Context, _ uint64, _ time.Time) error { return nil }

func (f *fakeStore) RevokeCLISessionsByUser(_ context.Context, _ uint64, _ time.Time) error {
	return nil
}

func (f *fakeStore) ListCLISessionsByUser(_ context.Context, _ uint64) ([]model.CLISession, error) {
	return nil, nil
}

func (f *fakeStore) GetUserByID(_ context.Context, id uint64) (*model.User, error) {
	user := f.users[id]
	if user == nil {
		return nil, nil
	}
	copied := *user
	return &copied, nil
}

func TestExchangeResponseIncludesUserEmail(t *testing.T) {
	store := newFakeStore()
	store.users[42] = &model.User{ID: 42, Email: "dev@example.com"}
	svc := NewService(store, "https://platform.example.com")

	verifier := "test-verifier"
	start, err := svc.Start(context.Background(), StartRequest{
		CodeChallenge:       expectedS256(verifier),
		CodeChallengeMethod: "S256",
		RedirectURI:         "http://127.0.0.1:12345/callback",
		State:               "state",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	_, code, err := svc.Complete(context.Background(), start.ChallengeID, 42)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	resp, err := svc.Exchange(context.Background(), ExchangeRequest{
		ChallengeID:  start.ChallengeID,
		Code:         code,
		CodeVerifier: verifier,
	})
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if resp.UserEmail != "dev@example.com" {
		t.Fatalf("UserEmail = %q", resp.UserEmail)
	}
}

func TestDeviceLoginFlowCreatesSessionWithoutLocalRedirect(t *testing.T) {
	store := newFakeStore()
	store.users[42] = &model.User{ID: 42, Email: "dev@example.com"}
	svc := NewService(store, "https://platform.example.com")

	verifier := "test-verifier"
	start, err := svc.Start(context.Background(), StartRequest{
		CodeChallenge:       expectedS256(verifier),
		CodeChallengeMethod: "S256",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if start.UserCode == "" {
		t.Fatalf("UserCode is empty")
	}
	if start.VerificationURL != "https://platform.example.com/api/cli/login/verify" {
		t.Fatalf("VerificationURL = %q", start.VerificationURL)
	}
	if want := "https://platform.example.com/api/cli/login/verify?user_code="; !strings.HasPrefix(start.LoginURL, want) {
		t.Fatalf("LoginURL = %q, want prefix %q", start.LoginURL, want)
	}

	challenge := store.challenges[start.ChallengeID]
	if challenge == nil {
		t.Fatalf("challenge was not stored")
	}
	if challenge.Flow != model.CLILoginChallengeFlowDevice {
		t.Fatalf("Flow = %q", challenge.Flow)
	}
	if challenge.UserCodeHash == nil || *challenge.UserCodeHash == "" {
		t.Fatalf("UserCodeHash was not stored")
	}

	if err := svc.VerifyUserCode(context.Background(), start.UserCode, 42); err != nil {
		t.Fatalf("VerifyUserCode: %v", err)
	}
	poll, err := svc.Poll(context.Background(), start.ChallengeID)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if poll.Status != model.CLILoginChallengeStatusCompleted {
		t.Fatalf("Poll status = %q", poll.Status)
	}

	resp, err := svc.Exchange(context.Background(), ExchangeRequest{
		ChallengeID:  start.ChallengeID,
		CodeVerifier: verifier,
	})
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if resp.Token == "" {
		t.Fatalf("Token is empty")
	}
	if resp.UserEmail != "dev@example.com" {
		t.Fatalf("UserEmail = %q", resp.UserEmail)
	}
}

func TestCallbackLoginFlowUsesProviderNeutralOAuth2Route(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, "https://platform.example.com")

	start, err := svc.Start(context.Background(), StartRequest{
		CodeChallenge:       expectedS256("test-verifier"),
		CodeChallengeMethod: "S256",
		RedirectURI:         "http://127.0.0.1:12345/callback",
		State:               "state",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !strings.HasPrefix(start.LoginURL, "https://platform.example.com/api/auth/oauth2/login?") {
		t.Fatalf("LoginURL = %q", start.LoginURL)
	}
	if !strings.Contains(start.LoginURL, "return_to=") {
		t.Fatalf("LoginURL missing return_to: %q", start.LoginURL)
	}
}
