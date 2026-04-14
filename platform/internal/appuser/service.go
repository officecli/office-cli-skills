package appuser

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	growthsvc "github.com/officecli/officecli/platform/internal/growth"
	"github.com/officecli/officecli/platform/internal/model"
	sqlstore "github.com/officecli/officecli/platform/internal/store/sqlstore"
)

type Store interface {
	CountUserAPIKeys(ctx context.Context, userID uint64) (int64, error)
	FindAPIKeysByOwner(ctx context.Context, userID uint64) ([]model.APIKey, error)
	ListAppUsageEvents(ctx context.Context, userID uint64) ([]model.UsageEvent, error)
	GetUserByID(ctx context.Context, id uint64) (*model.User, error)
	ListRewardGrantsByUser(ctx context.Context, userID uint64) ([]model.RewardGrant, error)
	ListReferralsByInviterUserID(ctx context.Context, inviterUserID uint64) ([]model.UserReferral, error)
	FindDiscordConnectionByUserID(ctx context.Context, userID uint64) (*model.DiscordConnection, error)
	CreateAuditLog(ctx context.Context, action, targetType, targetID string, payload string) error
	AppCreateAPIKey(ctx context.Context, userID uint64, planName, hash, prefix string) (*model.APIKey, error)
	UpdateAPIKey(ctx context.Context, id uint64, values map[string]any) error
	IsAPIKeyOwnedByUser(ctx context.Context, apiKeyID, userID uint64) (bool, error)
}

type Billing interface {
	ListOrdersByUser(ctx context.Context, userID uint64) ([]model.Order, error)
	Pricing() []model.PricingPack
}

type GrowthManager interface {
	ConnectDiscord(ctx context.Context, userID uint64, discordUserID, username string, guildMember bool) (*model.DiscordConnection, error)
	GrantDiscordJoinReward(ctx context.Context, userID uint64, rewardAmount int) (*growthsvc.RewardGrantResult, error)
}

const DiscordGuildVerificationBlockedReason = "discord guild verification is not configured in this build yet"

type Overview struct {
	APIKeyCount            int64               `json:"api_key_count"`
	TotalRemaining         int                 `json:"total_remaining"`
	RewardRemaining        int                 `json:"reward_remaining"`
	InviteCode             string              `json:"invite_code,omitempty"`
	InviteLimit            int                 `json:"invite_limit"`
	InviteRemaining        int                 `json:"invite_remaining"`
	RewardPerInvite        int                 `json:"reward_per_invite"`
	ReferralCount          int                 `json:"referral_count"`
	ActivatedReferralCount int                 `json:"activated_referral_count"`
	DiscordConnected       bool                `json:"discord_connected"`
	DiscordGuildMember     bool                `json:"discord_guild_member"`
	RecentUsageCount       int                 `json:"recent_usage_count"`
	RecentOrdersCount      int                 `json:"recent_orders_count"`
	Pricing                []model.PricingPack `json:"pricing"`
}

type APIKeyView struct {
	ID             uint64             `json:"id"`
	KeyPrefix      string             `json:"key_prefix"`
	Status         model.APIKeyStatus `json:"status"`
	PlanName       string             `json:"plan_name"`
	Note           *string            `json:"note,omitempty"`
	ExpiresAt      *time.Time         `json:"expires_at,omitempty"`
	LastUsedAt     *time.Time         `json:"last_used_at,omitempty"`
	QuotaTotal     *int               `json:"quota_total,omitempty"`
	QuotaUsed      int                `json:"quota_used"`
	QuotaRemaining int                `json:"quota_remaining"`
	CreatedAt      time.Time          `json:"created_at"`
}

type GrowthSnapshot struct {
	InviteCode        string                 `json:"invite_code,omitempty"`
	InviteLimit       int                    `json:"invite_limit"`
	InviteRemaining   int                    `json:"invite_remaining"`
	RewardPerInvite   int                    `json:"reward_per_invite"`
	RewardRemaining   int                    `json:"reward_remaining"`
	RewardGrants      []RewardGrantView      `json:"reward_grants"`
	Referrals         []ReferralView         `json:"referrals"`
	DiscordConnection *DiscordConnectionView `json:"discord_connection,omitempty"`
}

type RewardGrantView struct {
	SourceType   model.RewardSourceType `json:"source_type"`
	AmountTotal  int                    `json:"amount_total"`
	AmountUsed   int                    `json:"amount_used"`
	Remaining    int                    `json:"remaining"`
	Reason       string                 `json:"reason"`
	MetadataJSON string                 `json:"metadata_json"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

type ReferralView struct {
	InviteCode      string     `json:"invite_code"`
	RegisteredAt    time.Time  `json:"registered_at"`
	ActivatedAt     *time.Time `json:"activated_at,omitempty"`
	RewardGrantedAt *time.Time `json:"reward_granted_at,omitempty"`
}

type DiscordConnectionView struct {
	Username                  string     `json:"username"`
	GuildMember               bool       `json:"guild_member"`
	ConnectedAt               time.Time  `json:"connected_at"`
	RewardGrantedAt           *time.Time `json:"reward_granted_at,omitempty"`
	VerificationStatus        string     `json:"verification_status"`
	VerificationBlockedReason string     `json:"verification_blocked_reason,omitempty"`
}

type CreateAPIKeyRequest struct {
	PlanName string `json:"plan_name"`
}

type CreateAPIKeyResponse struct {
	PlaintextKey string     `json:"plaintext_key"`
	Key          APIKeyView `json:"key"`
}

type UpdateAPIKeyRequest struct {
	Status *string `json:"status,omitempty"`
	Note   *string `json:"note,omitempty"`
}

type ConnectDiscordRequest struct {
	DiscordUserID string `json:"discord_user_id"`
	Username      string `json:"username"`
}

type ConnectDiscordResponse struct {
	Connection           DiscordConnectionView `json:"connection"`
	RewardGranted        bool                  `json:"reward_granted"`
	RewardRemaining      int                   `json:"reward_remaining"`
	RewardSourceType     string                `json:"reward_source_type,omitempty"`
	RewardIdempotencyKey string                `json:"reward_idempotency_key,omitempty"`
}

type DiscordStatusResponse struct {
	Connection      *DiscordConnectionView `json:"connection,omitempty"`
	RewardRemaining int                    `json:"reward_remaining"`
}

type Service struct {
	store   Store
	billing Billing
	salt    string
	growth  GrowthManager
}

const defaultDiscordJoinRewardAmount = 2

func NewService(store Store, billing Billing, salt string, growth ...GrowthManager) *Service {
	var manager GrowthManager
	if len(growth) > 0 {
		manager = growth[0]
	}
	return &Service{store: store, billing: billing, salt: salt, growth: manager}
}

func (s *Service) Overview(ctx context.Context, userID uint64) (*Overview, error) {
	count, err := s.store.CountUserAPIKeys(ctx, userID)
	if err != nil {
		return nil, err
	}
	keys, err := s.store.FindAPIKeysByOwner(ctx, userID)
	if err != nil {
		return nil, err
	}
	usage, err := s.store.ListAppUsageEvents(ctx, userID)
	if err != nil {
		return nil, err
	}
	orders, err := s.billing.ListOrdersByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	rewardGrants, err := s.store.ListRewardGrantsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	referrals, err := s.store.ListReferralsByInviterUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	discordConnection, err := s.store.FindDiscordConnectionByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	remaining := 0
	for _, key := range keys {
		remaining += key.PaidQuotaRemaining()
	}
	visibleUsage := visibleUsageEvents(usage)
	visibleOrders := visibleOrders(orders)
	rewardRemaining := 0
	for _, grant := range rewardGrants {
		rewardRemaining += grant.Remaining()
	}
	activatedReferralCount := 0
	for _, referral := range referrals {
		if referral.ActivatedAt != nil {
			activatedReferralCount++
		}
	}
	inviteCount := len(referrals)
	inviteCode := ""
	if user != nil {
		inviteCode = user.InviteCode
	}
	return &Overview{
		APIKeyCount:            count,
		TotalRemaining:         remaining,
		RewardRemaining:        rewardRemaining,
		InviteCode:             inviteCode,
		InviteLimit:            growthsvc.MaxReferralsPerInviter,
		InviteRemaining:        inviteRemaining(inviteCount),
		RewardPerInvite:        growthsvc.InviteActivationRewardAmount,
		ReferralCount:          inviteCount,
		ActivatedReferralCount: activatedReferralCount,
		DiscordConnected:       discordConnection != nil,
		DiscordGuildMember:     discordConnection != nil && discordConnection.GuildMember,
		RecentUsageCount:       len(visibleUsage),
		RecentOrdersCount:      len(visibleOrders),
		Pricing:                externalPricingPacks(s.billing.Pricing()),
	}, nil
}

func (s *Service) ListAPIKeys(ctx context.Context, userID uint64) ([]APIKeyView, error) {
	keys, err := s.store.FindAPIKeysByOwner(ctx, userID)
	if err != nil {
		return nil, err
	}
	views := make([]APIKeyView, 0, len(keys))
	for _, key := range keys {
		views = append(views, newAPIKeyView(key))
	}
	return views, nil
}

func (s *Service) CreateAPIKey(ctx context.Context, userID uint64, req CreateAPIKeyRequest) (*CreateAPIKeyResponse, error) {
	planName := strings.TrimSpace(req.PlanName)
	if planName == "" {
		planName = "Starter"
	}
	plain, prefix, hash, err := generateAPIKey(s.salt)
	if err != nil {
		return nil, err
	}
	key, err := s.store.AppCreateAPIKey(ctx, userID, planName, hash, prefix)
	if err != nil {
		return nil, err
	}
	_ = s.store.CreateAuditLog(ctx, "app.api_key.create", "api_key", fmt.Sprintf("%d", key.ID), sqlstore.JSONString(key))
	return &CreateAPIKeyResponse{PlaintextKey: plain, Key: newAPIKeyView(*key)}, nil
}

func (s *Service) UpdateAPIKey(ctx context.Context, userID, apiKeyID uint64, req UpdateAPIKeyRequest) error {
	owned, err := s.store.IsAPIKeyOwnedByUser(ctx, apiKeyID, userID)
	if err != nil {
		return err
	}
	if !owned {
		return fmt.Errorf("api key not found")
	}
	updates := map[string]any{}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.Note != nil {
		updates["note"] = *req.Note
	}
	if len(updates) == 0 {
		return nil
	}
	if err := s.store.UpdateAPIKey(ctx, apiKeyID, updates); err != nil {
		return err
	}
	payload, _ := json.Marshal(updates)
	return s.store.CreateAuditLog(ctx, "app.api_key.update", "api_key", fmt.Sprintf("%d", apiKeyID), string(payload))
}

func (s *Service) ListUsageEvents(ctx context.Context, userID uint64) ([]model.UsageEvent, error) {
	events, err := s.store.ListAppUsageEvents(ctx, userID)
	if err != nil {
		return nil, err
	}
	visible := visibleUsageEvents(events)
	if visible == nil {
		return []model.UsageEvent{}, nil
	}
	return visible, nil
}

func (s *Service) Growth(ctx context.Context, userID uint64) (*GrowthSnapshot, error) {
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	rewardGrants, err := s.store.ListRewardGrantsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	referrals, err := s.store.ListReferralsByInviterUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	discordConnection, err := s.store.FindDiscordConnectionByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	snapshot := &GrowthSnapshot{
		InviteLimit:     growthsvc.MaxReferralsPerInviter,
		InviteRemaining: inviteRemaining(len(referrals)),
		RewardPerInvite: growthsvc.InviteActivationRewardAmount,
		RewardGrants:    make([]RewardGrantView, 0, len(rewardGrants)),
		Referrals:       make([]ReferralView, 0, len(referrals)),
	}
	if user != nil {
		snapshot.InviteCode = user.InviteCode
	}
	for _, grant := range rewardGrants {
		snapshot.RewardRemaining += grant.Remaining()
		snapshot.RewardGrants = append(snapshot.RewardGrants, RewardGrantView{
			SourceType:   grant.SourceType,
			AmountTotal:  grant.AmountTotal,
			AmountUsed:   grant.AmountUsed,
			Remaining:    grant.Remaining(),
			Reason:       grant.Reason,
			MetadataJSON: grant.MetadataJSON,
			CreatedAt:    grant.CreatedAt,
			UpdatedAt:    grant.UpdatedAt,
		})
	}
	for _, referral := range referrals {
		snapshot.Referrals = append(snapshot.Referrals, ReferralView{
			InviteCode:      referral.InviteCode,
			RegisteredAt:    referral.RegisteredAt,
			ActivatedAt:     referral.ActivatedAt,
			RewardGrantedAt: referral.RewardGrantedAt,
		})
	}
	if discordConnection != nil {
		view := newDiscordConnectionView(discordConnection)
		snapshot.DiscordConnection = &view
	}
	return snapshot, nil
}

func (s *Service) ConnectDiscord(ctx context.Context, userID uint64, req ConnectDiscordRequest) (*ConnectDiscordResponse, error) {
	if s.growth == nil {
		return nil, fmt.Errorf("discord growth service is not configured")
	}
	connection, err := s.growth.ConnectDiscord(ctx, userID, req.DiscordUserID, req.Username, false)
	if err != nil {
		return nil, err
	}

	response := &ConnectDiscordResponse{
		Connection: newDiscordConnectionView(connection),
	}

	if connection.GuildMember {
		result, err := s.growth.GrantDiscordJoinReward(ctx, userID, defaultDiscordJoinRewardAmount)
		if err != nil && err != growthsvc.ErrDiscordGuildRequired {
			return nil, err
		}
		if result != nil && result.Grant != nil {
			response.RewardGranted = result.Created
			response.RewardSourceType = string(result.Grant.SourceType)
			response.RewardIdempotencyKey = result.Grant.IdempotencyKey
			growthSnapshot, growthErr := s.Growth(ctx, userID)
			if growthErr == nil {
				response.RewardRemaining = growthSnapshot.RewardRemaining
			}
		}
	}

	growthSnapshot, growthErr := s.Growth(ctx, userID)
	if growthErr == nil {
		response.RewardRemaining = growthSnapshot.RewardRemaining
		if growthSnapshot.DiscordConnection != nil {
			response.Connection = *growthSnapshot.DiscordConnection
		}
	}

	return response, nil
}

func (s *Service) DiscordStatus(ctx context.Context, userID uint64) (*DiscordStatusResponse, error) {
	growthSnapshot, err := s.Growth(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &DiscordStatusResponse{
		Connection:      growthSnapshot.DiscordConnection,
		RewardRemaining: growthSnapshot.RewardRemaining,
	}, nil
}

func newDiscordConnectionView(connection *model.DiscordConnection) DiscordConnectionView {
	view := DiscordConnectionView{
		Username:        connection.Username,
		GuildMember:     connection.GuildMember,
		ConnectedAt:     connection.ConnectedAt,
		RewardGrantedAt: connection.RewardGrantedAt,
	}
	if connection.GuildMember {
		view.VerificationStatus = "verified"
		return view
	}
	view.VerificationStatus = "verification_blocked"
	view.VerificationBlockedReason = DiscordGuildVerificationBlockedReason
	return view
}

func inviteRemaining(count int) int {
	remaining := growthsvc.MaxReferralsPerInviter - count
	if remaining < 0 {
		return 0
	}
	return remaining
}

func externalPricingPacks(packs []model.PricingPack) []model.PricingPack {
	result := make([]model.PricingPack, 0, len(packs))
	for _, pack := range packs {
		if pack.PackKind != string(model.PackKindExternalGeneration) {
			continue
		}
		result = append(result, pack)
	}
	return result
}

func visibleOrders(orders []model.Order) []model.Order {
	result := make([]model.Order, 0, len(orders))
	for _, order := range orders {
		if order.PackKind != model.PackKindExternalGeneration {
			continue
		}
		result = append(result, order)
	}
	return result
}

func visibleUsageEvents(events []model.UsageEvent) []model.UsageEvent {
	result := make([]model.UsageEvent, 0, len(events))
	for _, event := range events {
		if event.Mode == model.UsageModeHosted {
			continue
		}
		result = append(result, event)
	}
	return result
}

func newAPIKeyView(key model.APIKey) APIKeyView {
	return APIKeyView{
		ID:             key.ID,
		KeyPrefix:      key.KeyPrefix,
		Status:         key.Status,
		PlanName:       key.PlanName,
		Note:           key.Note,
		ExpiresAt:      key.ExpiresAt,
		LastUsedAt:     key.LastUsedAt,
		QuotaTotal:     key.QuotaTotal,
		QuotaUsed:      key.QuotaUsed,
		QuotaRemaining: key.PaidQuotaRemaining(),
		CreatedAt:      key.CreatedAt,
	}
}

func generateAPIKey(salt string) (plain string, prefix string, hash string, err error) {
	buf := make([]byte, 24)
	if _, err = rand.Read(buf); err != nil {
		return "", "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	plain = "cop_live_" + token
	prefixLen := 12
	if len(plain) < prefixLen {
		prefixLen = len(plain)
	}
	prefix = plain[:prefixLen]
	sum := sha256.Sum256([]byte(strings.TrimSpace(salt) + ":" + plain))
	hash = hex.EncodeToString(sum[:])
	return plain, prefix, hash, nil
}
