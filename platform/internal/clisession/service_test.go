package clisession

import (
	"context"
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
