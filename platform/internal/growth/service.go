package growth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/officecli/officecli-internal/platform/internal/model"
)

var (
	ErrInviteCodeRequired       = errors.New("invite code is required")
	ErrInviterNotFound          = errors.New("inviter not found")
	ErrInviteLimitReached       = errors.New("invite limit reached")
	ErrSelfReferral             = errors.New("self referral is not allowed")
	ErrReferralNotFound         = errors.New("referral not found")
	ErrDiscordUserIDRequired    = errors.New("discord user id is required")
	ErrDiscordUsernameRequired  = errors.New("discord username is required")
	ErrDiscordConnectionMissing = errors.New("discord connection not found")
	ErrDiscordGuildRequired     = errors.New("discord guild membership is required")
	ErrUserAlreadyLinked        = errors.New("user already linked to another discord account")
	ErrDiscordAlreadyLinked     = errors.New("discord account already linked to another user")
	ErrInvalidRewardAmount      = errors.New("reward amount must be positive")
)

const (
	MaxReferralsPerInviter       = 5
	InviteActivationRewardAmount = 100
	DiscordJoinRewardAmount      = 100
)

type UserStore interface {
	FindUserByInviteCode(ctx context.Context, inviteCode string) (*model.User, error)
}

type ReferralStore interface {
	FindReferralByInvitedUserID(ctx context.Context, invitedUserID uint64) (*model.UserReferral, error)
	CountReferralsByInviterUserID(ctx context.Context, inviterUserID uint64) (int64, error)
	SaveReferral(ctx context.Context, referral *model.UserReferral) error
}

type referralStoreWithAtomicLimit interface {
	RegisterReferralWithinLimit(ctx context.Context, inviterUserID, invitedUserID uint64, inviteCode string, registeredAt time.Time) (*model.UserReferral, error)
}

type DiscordStore interface {
	FindDiscordConnectionByUserID(ctx context.Context, userID uint64) (*model.DiscordConnection, error)
	FindDiscordConnectionByDiscordUserID(ctx context.Context, discordUserID string) (*model.DiscordConnection, error)
	SaveDiscordConnection(ctx context.Context, connection *model.DiscordConnection) error
}

type RewardGrantStore interface {
	FindRewardGrantByIdempotencyKey(ctx context.Context, key string) (*model.RewardGrant, error)
	SaveRewardGrant(ctx context.Context, grant *model.RewardGrant) error
}

type HostedCreditStore interface {
	GrantHostedCreditsToUser(ctx context.Context, userID uint64, source model.HostedCreditLedgerSource, idempotencyKey string, creditAmount int, reason string, metadataJSON string) (*model.UserHostedCreditLedger, *model.UserHostedCreditAccount, bool, error)
}

type Service struct {
	users         UserStore
	referrals     ReferralStore
	discord       DiscordStore
	grants        RewardGrantStore
	hostedCredits HostedCreditStore
	clock         func() time.Time
}

type RewardGrantResult struct {
	Grant   *model.RewardGrant
	Created bool
}

func NewService(users UserStore, referrals ReferralStore, discord DiscordStore, grants RewardGrantStore, hostedCredits ...HostedCreditStore) *Service {
	var hostedStore HostedCreditStore
	if len(hostedCredits) > 0 {
		hostedStore = hostedCredits[0]
	}
	return &Service{
		users:         users,
		referrals:     referrals,
		discord:       discord,
		grants:        grants,
		hostedCredits: hostedStore,
		clock:         time.Now,
	}
}

func (s *Service) RegisterReferral(ctx context.Context, inviteCode string, invitedUserID uint64) (*model.UserReferral, error) {
	inviteCode = strings.TrimSpace(inviteCode)
	if inviteCode == "" {
		return nil, ErrInviteCodeRequired
	}

	existing, err := s.referrals.FindReferralByInvitedUserID(ctx, invitedUserID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	inviter, err := s.users.FindUserByInviteCode(ctx, inviteCode)
	if err != nil {
		return nil, err
	}
	if inviter == nil {
		return nil, ErrInviterNotFound
	}
	if inviter.ID == invitedUserID {
		return nil, ErrSelfReferral
	}
	if atomicStore, ok := s.referrals.(referralStoreWithAtomicLimit); ok {
		return atomicStore.RegisterReferralWithinLimit(ctx, inviter.ID, invitedUserID, inviteCode, s.now())
	}
	referralCount, err := s.referrals.CountReferralsByInviterUserID(ctx, inviter.ID)
	if err != nil {
		return nil, err
	}
	if referralCount >= MaxReferralsPerInviter {
		return nil, ErrInviteLimitReached
	}

	referral := &model.UserReferral{
		InviterUserID: inviter.ID,
		InvitedUserID: invitedUserID,
		InviteCode:    inviteCode,
		RegisteredAt:  s.now(),
	}
	if err := s.referrals.SaveReferral(ctx, referral); err != nil {
		return nil, err
	}
	return referral, nil
}

func (s *Service) ActivateReferral(ctx context.Context, invitedUserID uint64, rewardAmount int) (*RewardGrantResult, error) {
	if rewardAmount <= 0 {
		return nil, ErrInvalidRewardAmount
	}

	referral, err := s.referrals.FindReferralByInvitedUserID(ctx, invitedUserID)
	if err != nil {
		return nil, err
	}
	if referral == nil {
		return nil, ErrReferralNotFound
	}

	now := s.now()
	if referral.ActivatedAt == nil {
		referral.ActivatedAt = &now
	}

	idempotencyKey := fmt.Sprintf("invite_activation_bonus:%d:%d", referral.InviterUserID, invitedUserID)
	if s.hostedCredits != nil {
		created, err := s.grantHostedInviteCredits(ctx, referral, idempotencyKey, rewardAmount)
		if err != nil {
			return nil, err
		}
		if referral.RewardGrantedAt == nil {
			referral.RewardGrantedAt = &now
			if err := s.referrals.SaveReferral(ctx, referral); err != nil {
				return nil, err
			}
		}
		return &RewardGrantResult{Created: created}, nil
	}
	if existing, err := s.grants.FindRewardGrantByIdempotencyKey(ctx, idempotencyKey); err != nil {
		return nil, err
	} else if existing != nil {
		if referral.RewardGrantedAt == nil {
			grantedAt := existing.CreatedAt
			if grantedAt.IsZero() {
				grantedAt = now
			}
			referral.RewardGrantedAt = &grantedAt
			if err := s.referrals.SaveReferral(ctx, referral); err != nil {
				return nil, err
			}
		}
		return &RewardGrantResult{Grant: existing, Created: false}, nil
	}

	metadata, err := marshalMetadata(map[string]any{
		"invite_code":      referral.InviteCode,
		"inviter_user_id":  referral.InviterUserID,
		"invited_user_id":  referral.InvitedUserID,
		"activation_event": "first_successful_generation",
	})
	if err != nil {
		return nil, err
	}

	grant := &model.RewardGrant{
		UserID:         referral.InviterUserID,
		SourceType:     model.RewardSourceInviteActivation,
		IdempotencyKey: idempotencyKey,
		AmountTotal:    rewardAmount,
		Reason:         "invite activation reward",
		MetadataJSON:   metadata,
	}
	if err := s.grants.SaveRewardGrant(ctx, grant); err != nil {
		return nil, err
	}

	referral.RewardGrantedAt = &now
	if err := s.referrals.SaveReferral(ctx, referral); err != nil {
		return nil, err
	}
	return &RewardGrantResult{Grant: grant, Created: true}, nil
}

func (s *Service) grantHostedInviteCredits(ctx context.Context, referral *model.UserReferral, idempotencyKey string, rewardAmount int) (bool, error) {
	metadata, err := marshalMetadata(map[string]any{
		"invite_code":      referral.InviteCode,
		"inviter_user_id":  referral.InviterUserID,
		"invited_user_id":  referral.InvitedUserID,
		"activation_event": "first_successful_generation",
	})
	if err != nil {
		return false, err
	}
	_, _, created, err := s.hostedCredits.GrantHostedCreditsToUser(
		ctx,
		referral.InviterUserID,
		model.HostedCreditLedgerSourceInviteActivationBonus,
		idempotencyKey,
		rewardAmount,
		"invite activation hosted credits",
		metadata,
	)
	return created, err
}

func (s *Service) ConnectDiscord(ctx context.Context, userID uint64, discordUserID, username string, guildMember bool) (*model.DiscordConnection, error) {
	discordUserID = strings.TrimSpace(discordUserID)
	if discordUserID == "" {
		return nil, ErrDiscordUserIDRequired
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, ErrDiscordUsernameRequired
	}

	byUser, err := s.discord.FindDiscordConnectionByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if byUser != nil && byUser.DiscordUserID != discordUserID {
		return nil, ErrUserAlreadyLinked
	}

	byDiscord, err := s.discord.FindDiscordConnectionByDiscordUserID(ctx, discordUserID)
	if err != nil {
		return nil, err
	}
	if byDiscord != nil && byDiscord.UserID != userID {
		return nil, ErrDiscordAlreadyLinked
	}

	now := s.now()
	connection := byUser
	if connection == nil {
		connection = byDiscord
	}
	if connection == nil {
		connection = &model.DiscordConnection{
			UserID:        userID,
			DiscordUserID: discordUserID,
			Username:      username,
			GuildMember:   guildMember,
			ConnectedAt:   now,
		}
	} else {
		connection.DiscordUserID = discordUserID
		connection.Username = username
		connection.GuildMember = connection.GuildMember || guildMember
		if connection.ConnectedAt.IsZero() {
			connection.ConnectedAt = now
		}
	}

	if err := s.discord.SaveDiscordConnection(ctx, connection); err != nil {
		return nil, err
	}
	return connection, nil
}

func (s *Service) GrantDiscordJoinReward(ctx context.Context, userID uint64, rewardAmount int) (*RewardGrantResult, error) {
	if rewardAmount <= 0 {
		return nil, ErrInvalidRewardAmount
	}

	connection, err := s.discord.FindDiscordConnectionByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if connection == nil {
		return nil, ErrDiscordConnectionMissing
	}
	if !connection.GuildMember {
		return nil, ErrDiscordGuildRequired
	}

	now := s.now()
	idempotencyKey := fmt.Sprintf("discord_join_bonus:%d:%s", connection.UserID, connection.DiscordUserID)
	if s.hostedCredits != nil {
		metadata, err := marshalMetadata(map[string]any{
			"discord_user_id": connection.DiscordUserID,
			"guild_member":    connection.GuildMember,
			"username":        connection.Username,
		})
		if err != nil {
			return nil, err
		}
		_, _, created, err := s.hostedCredits.GrantHostedCreditsToUser(ctx, connection.UserID, model.HostedCreditLedgerSourceDiscordJoinBonus, idempotencyKey, rewardAmount, "discord guild join hosted credits", metadata)
		if err != nil {
			return nil, err
		}
		if connection.RewardGrantedAt == nil {
			connection.RewardGrantedAt = &now
			if err := s.discord.SaveDiscordConnection(ctx, connection); err != nil {
				return nil, err
			}
		}
		return &RewardGrantResult{Created: created}, nil
	}
	if existing, err := s.grants.FindRewardGrantByIdempotencyKey(ctx, idempotencyKey); err != nil {
		return nil, err
	} else if existing != nil {
		if connection.RewardGrantedAt == nil {
			grantedAt := existing.CreatedAt
			if grantedAt.IsZero() {
				grantedAt = now
			}
			connection.RewardGrantedAt = &grantedAt
			if err := s.discord.SaveDiscordConnection(ctx, connection); err != nil {
				return nil, err
			}
		}
		return &RewardGrantResult{Grant: existing, Created: false}, nil
	}

	metadata, err := marshalMetadata(map[string]any{
		"discord_user_id": connection.DiscordUserID,
		"guild_member":    connection.GuildMember,
		"username":        connection.Username,
	})
	if err != nil {
		return nil, err
	}

	grant := &model.RewardGrant{
		UserID:         connection.UserID,
		SourceType:     model.RewardSourceDiscordJoin,
		IdempotencyKey: idempotencyKey,
		AmountTotal:    rewardAmount,
		Reason:         "discord guild join reward",
		MetadataJSON:   metadata,
	}
	if err := s.grants.SaveRewardGrant(ctx, grant); err != nil {
		return nil, err
	}

	connection.RewardGrantedAt = &now
	if err := s.discord.SaveDiscordConnection(ctx, connection); err != nil {
		return nil, err
	}
	return &RewardGrantResult{Grant: grant, Created: true}, nil
}

func (s *Service) now() time.Time {
	if s.clock == nil {
		return time.Now().UTC()
	}
	return s.clock().UTC()
}

func marshalMetadata(payload map[string]any) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
