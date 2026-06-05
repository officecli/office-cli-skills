package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	redis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/officecli/officecli/platform/internal/apikey"
	"github.com/officecli/officecli/platform/internal/model"
	"github.com/officecli/officecli/platform/internal/redemption"
	redisstore "github.com/officecli/officecli/platform/internal/store/redis"
	sqlstore "github.com/officecli/officecli/platform/internal/store/sqlstore"
)

type fakeCodec struct{}

func (fakeCodec) Encode(v string) (string, error) { return v, nil }
func (fakeCodec) Decode(v string) (string, error) { return v, nil }

type fakeImageTemplateObjectStore struct {
	objects      map[string][]byte
	contentTypes map[string]string
}

func (f *fakeImageTemplateObjectStore) PutObject(_ context.Context, key string, reader io.Reader, _ int64, contentType string) error {
	if f.objects == nil {
		f.objects = map[string][]byte{}
	}
	if f.contentTypes == nil {
		f.contentTypes = map[string]string{}
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	f.objects[key] = data
	f.contentTypes[key] = contentType
	return nil
}

func (f *fakeImageTemplateObjectStore) GetObject(_ context.Context, key string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(f.objects[key])), nil
}

type memoryRedis struct{ data map[string]any }

func TestNewServiceNormalizesAdminAllowlist(t *testing.T) {
	cipher, err := apikey.NewCipher(apikey.DefaultDevEncryptionKey)
	require.NoError(t, err)
	svc := NewService(nil, nil, "secret", time.Hour, "cookie", fakeCodec{}, "salt", cipher, nil, []string{
		"  LUYANG950@GMAIL.COM  ",
		"",
		"luyang950@gmail.com",
	})

	require.Len(t, svc.adminAllowlist, 1)
	_, ok := svc.adminAllowlist["luyang950@gmail.com"]
	require.True(t, ok)
	_, ok = svc.adminAllowlist["LUYANG950@GMAIL.COM"]
	require.False(t, ok)
}

func TestNewServiceRejectsNonAllowlistedEmailAfterNormalization(t *testing.T) {
	cipher, err := apikey.NewCipher(apikey.DefaultDevEncryptionKey)
	require.NoError(t, err)
	svc := NewService(nil, nil, "secret", time.Hour, "cookie", fakeCodec{}, "salt", cipher, nil, []string{
		" luyang950@gmail.com ",
	})

	_, allowed := svc.adminAllowlist["someone@example.com"]
	require.False(t, allowed)
	_, allowed = svc.adminAllowlist["luyang950@gmail.com"]
	require.True(t, allowed)
}

func TestServiceMockDataProvidesAdminListSamples(t *testing.T) {
	cipher, err := apikey.NewCipher(apikey.DefaultDevEncryptionKey)
	require.NoError(t, err)
	svc := NewService(nil, nil, "secret", time.Hour, "cookie", fakeCodec{}, "salt", cipher, nil, nil)
	svc.UseMockData(true)

	users, err := svc.ListUsers(context.Background(), "")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(users), 3)

	filteredUsers, err := svc.ListUsers(context.Background(), "disabled")
	require.NoError(t, err)
	require.Len(t, filteredUsers, 1)
	require.Equal(t, model.UserStatusDisabled, filteredUsers[0].Status)

	keys, err := svc.ListAPIKeys(context.Background(), nil)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(keys), 3)
	require.NotNil(t, keys[0].OwnerUserID)

	ownerID := *keys[0].OwnerUserID
	ownedKeys, err := svc.ListAPIKeys(context.Background(), &ownerID)
	require.NoError(t, err)
	require.NotEmpty(t, ownedKeys)
	for _, key := range ownedKeys {
		require.NotNil(t, key.OwnerUserID)
		require.Equal(t, ownerID, *key.OwnerUserID)
	}

	events, err := svc.ListUsageEvents(context.Background(), sqlstore.UsageEventFilter{Result: string(model.UsageResultBlocked)})
	require.NoError(t, err)
	require.NotEmpty(t, events)
	for _, event := range events {
		require.Equal(t, model.UsageResultBlocked, event.Result)
	}

	overview, err := svc.Overview(context.Background())
	require.NoError(t, err)
	require.Greater(t, overview.TotalUsers, int64(0))
	require.Greater(t, overview.TotalAPIKeys, int64(0))

	sources, err := svc.QuotaSources(context.Background(), QuotaSourcesFilter{})
	require.NoError(t, err)
	require.NotEmpty(t, sources.RewardGrants)
	require.NotEmpty(t, sources.PaidExternalKeys)
	require.NotEmpty(t, sources.HostedKeys)
}

func TestCreateKeyAndUpdateQuota(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.APIKey{}, &model.UsageEvent{}, &model.AdminAuditLog{}))
	store := sqlstore.NewWithDB(db)
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	_ = client
	redisRepo := redisstore.NewStore(redis.NewClient(&redis.Options{Addr: "localhost:6379"}))
	cipher, err := apikey.NewCipher(apikey.DefaultDevEncryptionKey)
	require.NoError(t, err)
	svc := NewService(store, redisRepo, "secret", time.Hour, "cookie", fakeCodec{}, "salt", cipher, nil, nil)

	note := "created by test"
	quotaTotal := 20
	result, key, err := svc.CreateAPIKey(context.Background(), CreateAPIKeyRequest{PlanName: "pro", Note: &note, QuotaTotal: &quotaTotal})
	require.NoError(t, err)
	require.NotEmpty(t, result.PlaintextKey)
	require.NotEmpty(t, result.KeyPrefix)

	keys, err := svc.ListAPIKeys(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Equal(t, key.KeyPrefix, keys[0].KeyPrefix)
	require.True(t, keys[0].PlaintextAvailable)
	require.Equal(t, model.APIKeyStatusActive, keys[0].Status)
	require.Equal(t, "pro", keys[0].PlanName)
	require.NotNil(t, keys[0].QuotaTotal)
	require.Equal(t, 20, *keys[0].QuotaTotal)

	plaintext, err := svc.GetAPIKeyPlaintext(context.Background(), key.ID, "admin@example.com")
	require.NoError(t, err)
	require.Equal(t, result.PlaintextKey, plaintext)
}

func TestDeleteRedemptionCodeRemovesCodeAndAudits(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:delete_redemption_code?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.RedemptionCode{}, &model.AdminAuditLog{}))
	store := sqlstore.NewWithDB(db)
	cipher, err := apikey.NewCipher(apikey.DefaultDevEncryptionKey)
	require.NoError(t, err)
	svc := NewService(store, nil, "secret", time.Hour, "cookie", fakeCodec{}, "salt", cipher, nil, nil)
	svc.SetRedemptionService(redemption.NewService(store))

	created, err := svc.CreateRedemptionCode(context.Background(), "admin@example.com", CreateRedemptionCodeRequest{
		Code:         "DELETE-ME",
		CreditAmount: 25,
	})
	require.NoError(t, err)

	require.NoError(t, svc.DeleteRedemptionCode(context.Background(), "admin@example.com", created.ID))
	_, err = store.GetRedemptionCodeByID(context.Background(), created.ID)
	require.ErrorIs(t, err, sqlstore.ErrRedemptionCodeNotFound)

	var logs []model.AdminAuditLog
	require.NoError(t, db.Order("id ASC").Find(&logs).Error)
	require.Len(t, logs, 2)
	require.Equal(t, "redemption_code.delete", logs[1].Action)
	require.Equal(t, "redemption_code", logs[1].TargetType)
	require.JSONEq(t, `{"actor":"admin@example.com"}`, logs[1].PayloadJSON)
}

func TestCreateAPIKeyPersistsHostedOnlyFields(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:create_hosted_only_key?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.APIKey{}, &model.AdminAuditLog{}))
	store := sqlstore.NewWithDB(db)
	cipher, err := apikey.NewCipher(apikey.DefaultDevEncryptionKey)
	require.NoError(t, err)
	svc := NewService(store, nil, "secret", time.Hour, "cookie", fakeCodec{}, "salt", cipher, nil, nil)
	user := &model.User{GoogleSub: model.StringPtr("hosted-owner"), Email: "owner@example.com", Name: "Owner", InviteCode: "invite-owner", Status: model.UserStatusActive}
	require.NoError(t, db.Create(user).Error)

	allowedModes := "hosted_only"
	hostedEnabled := true
	defaultRuntimeMode := "hosted"
	creditBalance := 120
	result, key, err := svc.CreateAPIKey(context.Background(), CreateAPIKeyRequest{
		OwnerUserID:        &user.ID,
		PlanName:           "Hosted",
		AllowedModes:       &allowedModes,
		HostedEnabled:      &hostedEnabled,
		DefaultRuntimeMode: &defaultRuntimeMode,
		CreditBalance:      &creditBalance,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.PlaintextKey)

	saved, err := store.FindAPIKeyByID(context.Background(), key.ID)
	require.NoError(t, err)
	require.Equal(t, "hosted_only", saved.AllowedModes)
	require.True(t, saved.HostedEnabled)
	require.NotNil(t, saved.DefaultRuntimeMode)
	require.Equal(t, "hosted", *saved.DefaultRuntimeMode)
	require.Equal(t, 120, saved.CreditBalance)
	require.Nil(t, saved.QuotaTotal)
}

func TestCreateAPIKeyRequiresOwnerForHostedModes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:create_hosted_key_requires_owner?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.APIKey{}, &model.AdminAuditLog{}))
	store := sqlstore.NewWithDB(db)
	cipher, err := apikey.NewCipher(apikey.DefaultDevEncryptionKey)
	require.NoError(t, err)
	svc := NewService(store, nil, "secret", time.Hour, "cookie", fakeCodec{}, "salt", cipher, nil, nil)

	hostedOnly := "hosted_only"
	hostedEnabled := true
	_, _, err = svc.CreateAPIKey(context.Background(), CreateAPIKeyRequest{
		PlanName:      "Hosted",
		AllowedModes:  &hostedOnly,
		HostedEnabled: &hostedEnabled,
	})
	require.ErrorContains(t, err, "owner_user_id is required")

	hybrid := "hybrid"
	_, _, err = svc.CreateAPIKey(context.Background(), CreateAPIKeyRequest{
		PlanName:     "Hybrid",
		AllowedModes: &hybrid,
	})
	require.ErrorContains(t, err, "owner_user_id is required")
}

func TestCreateAPIKeyRejectsMissingHostedOwnerUser(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:create_hosted_key_rejects_missing_owner?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.APIKey{}, &model.AdminAuditLog{}))
	store := sqlstore.NewWithDB(db)
	cipher, err := apikey.NewCipher(apikey.DefaultDevEncryptionKey)
	require.NoError(t, err)
	svc := NewService(store, nil, "secret", time.Hour, "cookie", fakeCodec{}, "salt", cipher, nil, nil)

	missingUserID := uint64(404)
	hostedOnly := "hosted_only"
	hostedEnabled := true
	_, _, err = svc.CreateAPIKey(context.Background(), CreateAPIKeyRequest{
		OwnerUserID:   &missingUserID,
		PlanName:      "Hosted",
		AllowedModes:  &hostedOnly,
		HostedEnabled: &hostedEnabled,
	})
	require.ErrorContains(t, err, "owner user not found")
}

func TestCreateAPIKeyAllowsExternalWithoutOwner(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:create_external_key_without_owner?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.APIKey{}, &model.AdminAuditLog{}))
	store := sqlstore.NewWithDB(db)
	cipher, err := apikey.NewCipher(apikey.DefaultDevEncryptionKey)
	require.NoError(t, err)
	svc := NewService(store, nil, "secret", time.Hour, "cookie", fakeCodec{}, "salt", cipher, nil, nil)

	externalOnly := "external_only"
	hostedEnabled := false
	result, key, err := svc.CreateAPIKey(context.Background(), CreateAPIKeyRequest{
		PlanName:      "External",
		AllowedModes:  &externalOnly,
		HostedEnabled: &hostedEnabled,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.PlaintextKey)
	require.Nil(t, key.OwnerUserID)
}

func TestUpdateAPIKeyPersistsHostedEntitlementFields(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:update_hosted_entitlement_fields?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.APIKey{}, &model.AdminAuditLog{}))
	store := sqlstore.NewWithDB(db)
	svc := NewService(store, nil, "secret", time.Hour, "cookie", fakeCodec{}, "salt", nil, nil, nil)

	defaultRuntimeMode := "external"
	key := &model.APIKey{
		KeyHash:            "hash-hosted-update",
		KeyPrefix:          "cop_hosted_update",
		Status:             model.APIKeyStatusActive,
		PlanName:           "External",
		AllowedModes:       "external_only",
		HostedEnabled:      false,
		DefaultRuntimeMode: &defaultRuntimeMode,
	}
	require.NoError(t, db.Create(key).Error)

	allowedModes := "hosted_only"
	hostedEnabled := true
	hostedRuntime := "hosted"
	creditBalance := 75
	require.NoError(t, svc.UpdateAPIKey(context.Background(), key.ID, UpdateAPIKeyRequest{
		AllowedModes:       &allowedModes,
		HostedEnabled:      &hostedEnabled,
		DefaultRuntimeMode: &hostedRuntime,
		CreditBalance:      &creditBalance,
	}))

	saved, err := store.FindAPIKeyByID(context.Background(), key.ID)
	require.NoError(t, err)
	require.Equal(t, "hosted_only", saved.AllowedModes)
	require.True(t, saved.HostedEnabled)
	require.NotNil(t, saved.DefaultRuntimeMode)
	require.Equal(t, "hosted", *saved.DefaultRuntimeMode)
	require.Equal(t, 75, saved.CreditBalance)
}

func TestHostedPricingSettingsRulesAndPacksAreEditable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:hosted_pricing_admin?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.HostedPricingSetting{},
		&model.HostedModelPricingConfig{},
		&model.HostedPricingRule{},
		&model.HostedCreditPack{},
		&model.AdminAuditLog{},
	))
	store := sqlstore.NewWithDB(db)
	svc := NewService(store, nil, "secret", time.Hour, "cookie", fakeCodec{}, "salt", nil, nil, nil)

	settings, err := svc.UpdateHostedPricingSettings(context.Background(), UpdateHostedPricingSettingsRequest{MarkupBPS: 3500, CreditsPerUSD: 100})
	require.NoError(t, err)
	require.Equal(t, 3500, settings.MarkupBPS)
	require.Equal(t, "usd", settings.Currency)
	require.Equal(t, 100, settings.CreditsPerUSD)

	override := 5000
	modelConfig, err := svc.CreateHostedModelPricingConfig(context.Background(), UpsertHostedModelPricingConfigRequest{
		Key:                       "text_default",
		Kind:                      string(model.HostedModelPricingKindText),
		Provider:                  "aigateway",
		Model:                     "gpt-shared-text",
		PromptPer1MCostCredits:    ptrInt64(100),
		OutputPer1MCostCredits:    ptrInt64(200),
		ReasoningPer1MCostCredits: ptrInt64(300),
		Enabled:                   true,
	})
	require.NoError(t, err)
	require.Equal(t, "text_default", modelConfig.Key)
	require.Equal(t, int64(1000000), modelConfig.PromptPer1MCostMicrousd)
	require.Equal(t, int64(100), modelConfig.PromptPer1MCostCredits)

	rule, err := svc.CreateHostedPricingRule(context.Background(), UpsertHostedPricingRuleRequest{
		DocumentProfile:            "text",
		Provider:                   "aigateway",
		Model:                      "gpt-test",
		TextModelKey:               "text_default",
		PromptPer1KCostMicrousd:    10000,
		OutputPer1KCostMicrousd:    20000,
		ReasoningPer1KCostMicrousd: 40000,
		ImagePerAssetCostMicrousd:  0,
		MinimumChargeCredits:       2,
		MarkupBPS:                  &override,
		Enabled:                    true,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(1), rule.ID)
	require.NotNil(t, rule.MarkupBPS)
	require.Equal(t, 5000, *rule.MarkupBPS)
	require.Equal(t, "text_default", rule.TextModelKey)

	updatedModelConfig, err := svc.UpdateHostedModelPricingConfig(context.Background(), modelConfig.ID, UpsertHostedModelPricingConfigRequest{
		Key:                       "text_default",
		Kind:                      string(model.HostedModelPricingKindText),
		Provider:                  "aigateway",
		Model:                     "gpt-shared-text-2",
		PromptPer1MCostCredits:    ptrInt64(110),
		OutputPer1MCostCredits:    ptrInt64(210),
		ReasoningPer1MCostCredits: ptrInt64(310),
		Enabled:                   true,
	})
	require.NoError(t, err)
	require.Equal(t, "gpt-shared-text-2", updatedModelConfig.Model)
	require.Equal(t, int64(110), updatedModelConfig.PromptPer1MCostCredits)
	require.Equal(t, int64(1100000), updatedModelConfig.PromptPer1MCostMicrousd)

	pack, err := svc.CreateHostedCreditPack(context.Background(), UpsertHostedCreditPackRequest{
		Code:         "hosted-300",
		Name:         "Hosted 300",
		Description:  "300 hosted credits",
		Currency:     "usd",
		CreditAmount: 300,
		Enabled:      true,
	})
	require.NoError(t, err)
	require.Equal(t, "hosted-300", pack.Code)
	require.Equal(t, int64(300), pack.AmountTotal)

	payload, err := svc.HostedBillingConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, 3500, payload.Settings.MarkupBPS)
	require.Equal(t, 100, payload.Settings.CreditsPerUSD)
	require.Len(t, payload.ModelConfigs, 2)
	require.Equal(t, "image_default", payload.ModelConfigs[0].Key)
	require.Equal(t, "text_default", payload.ModelConfigs[1].Key)
	require.Equal(t, int64(110), payload.ModelConfigs[1].PromptPer1MCostCredits)
	require.Len(t, payload.Rules, 1)
	require.Len(t, payload.Packs, 1)
	require.True(t, payload.Packs[0].Enabled)

	var auditCount int64
	require.NoError(t, db.Model(&model.AdminAuditLog{}).Count(&auditCount).Error)
	require.Equal(t, int64(5), auditCount)
}

func TestImagePromptTemplatesServiceCRUDThumbnailAndCompose(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:image_prompt_template_service?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ImagePromptTemplate{}, &model.AdminAuditLog{}))

	store := sqlstore.NewWithDB(db)
	svc := NewService(store, nil, "secret", time.Hour, "cookie", fakeCodec{}, "salt", nil, nil, nil)
	objectStore := &fakeImageTemplateObjectStore{}
	svc.SetImageTemplateObjectStore(objectStore)

	created, err := svc.CreateImagePromptTemplate(context.Background(), UpsertImagePromptTemplateRequest{
		Slug:         "poster",
		Title:        "Poster",
		Description:  "Poster template",
		PromptPreset: "cinematic poster style",
		SortOrder:    10,
		Enabled:      true,
	})
	require.NoError(t, err)
	require.Equal(t, "poster", created.Slug)
	require.Equal(t, "cinematic poster style", created.PromptPreset)

	publicItems, err := svc.ListImagePromptTemplates(context.Background(), true, "/api/image-templates")
	require.NoError(t, err)
	require.Len(t, publicItems, 1)
	require.Empty(t, publicItems[0].ThumbnailURL)
	require.Equal(t, "cinematic poster style", publicItems[0].PromptPreset)

	composed, err := svc.ComposeImagePromptTemplate(context.Background(), created.ID, "red bicycle")
	require.NoError(t, err)
	require.Contains(t, composed.Prompt, "cinematic poster style")
	require.Contains(t, composed.Prompt, "red bicycle")

	withThumbnail, err := svc.UploadImagePromptTemplateThumbnail(context.Background(), created.ID, "thumb.png", "application/octet-stream", bytes.NewReader([]byte("png")), 3)
	require.NoError(t, err)
	require.Contains(t, withThumbnail.ThumbnailURL, "/thumbnail")
	require.Len(t, objectStore.objects, 1)
	var storageKey string
	for key := range objectStore.objects {
		storageKey = key
	}
	require.Equal(t, "image/png", objectStore.contentTypes[storageKey])

	publicItems, err = svc.ListImagePromptTemplates(context.Background(), true, "/api/image-templates")
	require.NoError(t, err)
	require.Len(t, publicItems, 1)
	require.Contains(t, publicItems[0].ThumbnailURL, "/api/image-templates/")
	require.Contains(t, publicItems[0].ThumbnailURL, "/thumbnail")
	require.Equal(t, "cinematic poster style", publicItems[0].PromptPreset)

	body, contentType, err := svc.ReadImagePromptTemplateThumbnail(context.Background(), created.ID)
	require.NoError(t, err)
	defer body.Close()
	require.Equal(t, "image/png", contentType)
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	require.Equal(t, []byte("png"), data)

	disabled, err := svc.SetImagePromptTemplateEnabled(context.Background(), created.ID, false)
	require.NoError(t, err)
	require.False(t, disabled.Enabled)
	_, err = svc.ComposeImagePromptTemplate(context.Background(), created.ID, "red bicycle")
	require.Error(t, err)
	require.Contains(t, err.Error(), "disabled")
}

func ptrInt64(value int64) *int64 {
	return &value
}

func TestImagePromptTemplateSlotsRoundTripListAndValidation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:image_prompt_template_slots?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ImagePromptTemplate{}, &model.AdminAuditLog{}))

	store := sqlstore.NewWithDB(db)
	svc := NewService(store, nil, "secret", time.Hour, "cookie", fakeCodec{}, "salt", nil, nil, nil)

	slots := []ImagePromptSlot{
		{Key: "product", Label: "Product", Example: "running shoes", DefaultValue: "a product", Required: true},
		{Key: "mood", Label: "Mood", Multiline: true},
	}
	created, err := svc.CreateImagePromptTemplate(context.Background(), UpsertImagePromptTemplateRequest{
		Slug:         "poster-slots",
		Title:        "Poster",
		Description:  "Poster template",
		PromptPreset: "A {{product}} poster, mood: {{mood}}",
		SortOrder:    10,
		Enabled:      true,
		Slots:        slots,
	})
	require.NoError(t, err)
	require.Len(t, created.Slots, 2)
	require.Equal(t, "product", created.Slots[0].Key)
	require.True(t, created.Slots[0].Required)
	require.True(t, created.Slots[1].Multiline)

	// Admin round-trip: reload via list and confirm slots survive persistence.
	adminItems, err := svc.ListAdminImagePromptTemplates(context.Background(), "/api/image-templates")
	require.NoError(t, err)
	require.Len(t, adminItems, 1)
	require.Len(t, adminItems[0].Slots, 2)
	require.Equal(t, "mood", adminItems[0].Slots[1].Key)

	publicItems, err := svc.ListImagePromptTemplates(context.Background(), true, "/api/image-templates")
	require.NoError(t, err)
	require.Len(t, publicItems, 1)
	require.Len(t, publicItems[0].Slots, 2)

	// Legacy concat path must ignore slots entirely — markers stay literal,
	// no slot substitution happens server-side (frontend-only assembly).
	composed, err := svc.ComposeImagePromptTemplate(context.Background(), created.ID, "red bicycle")
	require.NoError(t, err)
	require.Contains(t, composed.Prompt, "{{product}}")
	require.Contains(t, composed.Prompt, "{{mood}}")
	require.Contains(t, composed.Prompt, "red bicycle")

	// Updating with empty slots rolls the template back to legacy behavior.
	updated, err := svc.UpdateImagePromptTemplate(context.Background(), created.ID, UpsertImagePromptTemplateRequest{
		Slug:         "poster-slots",
		Title:        "Poster",
		Description:  "Poster template",
		PromptPreset: "A {{product}} poster, mood: {{mood}}",
		SortOrder:    10,
		Enabled:      true,
	})
	require.NoError(t, err)
	require.Empty(t, updated.Slots)

	// Validation: invalid key shape and duplicate keys are rejected.
	_, err = svc.CreateImagePromptTemplate(context.Background(), UpsertImagePromptTemplateRequest{
		Slug: "bad-key", Title: "Bad", PromptPreset: "x", Enabled: true,
		Slots: []ImagePromptSlot{{Key: "Bad-Key", Label: "Bad"}},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "^[a-z0-9_]+$")

	_, err = svc.CreateImagePromptTemplate(context.Background(), UpsertImagePromptTemplateRequest{
		Slug: "dup-key", Title: "Dup", PromptPreset: "x", Enabled: true,
		Slots: []ImagePromptSlot{{Key: "a", Label: "A"}, {Key: "a", Label: "A2"}},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicated")
}

func TestImagePromptTemplateOldShapeWithoutSlots(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:image_prompt_template_old_shape?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ImagePromptTemplate{}, &model.AdminAuditLog{}))

	store := sqlstore.NewWithDB(db)
	svc := NewService(store, nil, "secret", time.Hour, "cookie", fakeCodec{}, "salt", nil, nil, nil)

	// An old client payload carries no "slots" field at all; it must decode and persist fine.
	var req UpsertImagePromptTemplateRequest
	require.NoError(t, json.Unmarshal([]byte(`{"slug":"legacy","title":"Legacy","prompt_preset":"plain preset","sort_order":1,"enabled":true}`), &req))
	require.Nil(t, req.Slots)

	created, err := svc.CreateImagePromptTemplate(context.Background(), req)
	require.NoError(t, err)
	require.Empty(t, created.Slots)

	items, err := svc.ListImagePromptTemplates(context.Background(), true, "/api/image-templates")
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Empty(t, items[0].Slots)
}

func TestUserPrivateImageTemplateCopyAndPublishReview(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:image_prompt_template_publish_flow?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ImagePromptTemplate{}, &model.ImageGenerationProvenance{}, &model.ImageTemplatePublishRequest{}, &model.AdminAuditLog{}))

	store := sqlstore.NewWithDB(db)
	svc := NewService(store, nil, "secret", time.Hour, "cookie", fakeCodec{}, "salt", nil, nil, nil)
	objectStore := &fakeImageTemplateObjectStore{}
	svc.SetImageTemplateObjectStore(objectStore)

	public, err := svc.CreateImagePromptTemplate(context.Background(), UpsertImagePromptTemplateRequest{
		Slug:         "public-poster",
		Title:        "Public Poster",
		Description:  "Platform template",
		PromptPreset: "Public preset for {{product}}",
		SortOrder:    10,
		Enabled:      true,
		Slots: []ImagePromptSlot{
			{Key: "product", Label: "Product", Required: true},
		},
	})
	require.NoError(t, err)

	privateCopy, err := svc.CreateUserImagePromptTemplate(context.Background(), 42, CreateUserImagePromptTemplateRequest{
		SourceTemplateID: public.ID,
		Slug:             "my-poster",
		Title:            "My Poster",
		Description:      "Saved in my account",
	})
	require.NoError(t, err)
	require.Equal(t, "my-poster", privateCopy.Slug)
	require.Equal(t, "Public preset for {{product}}", privateCopy.PromptPreset)
	require.Equal(t, uint64(42), privateCopy.OwnerUserID)
	require.Equal(t, "user_private", privateCopy.Visibility)
	require.Len(t, privateCopy.Slots, 1)

	otherUserItems, err := svc.ListUserImagePromptTemplates(context.Background(), 7, "/api/image-templates")
	require.NoError(t, err)
	require.Empty(t, otherUserItems.Private)
	require.Len(t, otherUserItems.Public, 1)

	provenance, err := svc.RecordImageGenerationProvenance(context.Background(), RecordImageGenerationProvenanceRequest{
		RequestID:   "img-req-1",
		UserID:      42,
		Prompt:      privateCopy.PromptPreset,
		ImageName:   "poster.png",
		ContentType: "image/png",
		Image:       bytes.NewReader([]byte("generated-image")),
		ImageSize:   int64(len("generated-image")),
	})
	require.NoError(t, err)
	require.NotZero(t, provenance.ID)
	require.NotEmpty(t, provenance.PromptSHA256)
	require.NotEmpty(t, provenance.ImageSHA256)

	submission, err := svc.CreateImageTemplatePublishRequest(context.Background(), 42, CreateImageTemplatePublishRequest{
		PrivateTemplateID: privateCopy.ID,
		RequestID:         provenance.RequestID,
		SubmitterNote:     "Useful launch poster",
	})
	require.NoError(t, err)
	require.Equal(t, "pending", submission.Status)
	require.Equal(t, privateCopy.ID, submission.PrivateTemplateID)

	approved, err := svc.ReviewImageTemplatePublishRequest(context.Background(), "admin@example.com", submission.ID, ReviewImageTemplatePublishRequest{
		Action:     "approve",
		AdminNotes: "Looks good",
	})
	require.NoError(t, err)
	require.Equal(t, "approved", approved.Status)
	require.NotZero(t, approved.PublicTemplateID)

	publicItems, err := svc.ListImagePromptTemplates(context.Background(), true, "/api/image-templates")
	require.NoError(t, err)
	require.Len(t, publicItems, 2)
	var published *ImagePromptTemplateResponse
	for i := range publicItems {
		if publicItems[i].Title == "My Poster" {
			published = &publicItems[i]
			break
		}
	}
	require.NotNil(t, published)
	require.Equal(t, "Public preset for {{product}}", published.PromptPreset)
	require.Contains(t, published.ThumbnailURL, "/api/image-templates/")
}

func TestImageTemplatePublishRequestRejectsMismatchedProvenance(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:image_prompt_template_publish_rejects_mismatch?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ImagePromptTemplate{}, &model.ImageGenerationProvenance{}, &model.ImageTemplatePublishRequest{}, &model.AdminAuditLog{}))

	store := sqlstore.NewWithDB(db)
	svc := NewService(store, nil, "secret", time.Hour, "cookie", fakeCodec{}, "salt", nil, nil, nil)
	svc.SetImageTemplateObjectStore(&fakeImageTemplateObjectStore{})

	privateTemplate, err := svc.CreateUserImagePromptTemplate(context.Background(), 42, CreateUserImagePromptTemplateRequest{
		Slug:         "private",
		Title:        "Private",
		PromptPreset: "expected prompt",
	})
	require.NoError(t, err)
	otherPrompt, err := svc.RecordImageGenerationProvenance(context.Background(), RecordImageGenerationProvenanceRequest{
		RequestID:   "img-req-2",
		UserID:      42,
		Prompt:      "tampered prompt",
		ImageName:   "poster.png",
		ContentType: "image/png",
		Image:       bytes.NewReader([]byte("generated-image")),
		ImageSize:   int64(len("generated-image")),
	})
	require.NoError(t, err)

	_, err = svc.CreateImageTemplatePublishRequest(context.Background(), 42, CreateImageTemplatePublishRequest{
		PrivateTemplateID: privateTemplate.ID,
		ProvenanceID:      otherPrompt.ID,
	})
	require.ErrorContains(t, err, "generated prompt does not match private template")

	otherUser, err := svc.RecordImageGenerationProvenance(context.Background(), RecordImageGenerationProvenanceRequest{
		RequestID:   "img-req-3",
		UserID:      7,
		Prompt:      "expected prompt",
		ImageName:   "poster.png",
		ContentType: "image/png",
		Image:       bytes.NewReader([]byte("other-user-image")),
		ImageSize:   int64(len("other-user-image")),
	})
	require.NoError(t, err)
	_, err = svc.CreateImageTemplatePublishRequest(context.Background(), 42, CreateImageTemplatePublishRequest{
		PrivateTemplateID: privateTemplate.ID,
		ProvenanceID:      otherUser.ID,
	})
	require.ErrorContains(t, err, "generation task does not belong to user")
}

func TestParseImagePromptSlotsLegacyShapes(t *testing.T) {
	// A row predating the slots column reads back as "" → no slots, no panic.
	require.Nil(t, parseImagePromptSlots(""))
	require.Nil(t, parseImagePromptSlots("[]"))
	require.Nil(t, parseImagePromptSlots("not json"))

	slots := parseImagePromptSlots(`[{"key":"product","label":"Product","required":true}]`)
	require.Len(t, slots, 1)
	require.Equal(t, "product", slots[0].Key)
	require.True(t, slots[0].Required)
}

func TestUpdateUserDisablesOwnedAPIKeysWhenUserIsDisabled(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:update_user_disables_owned_api_keys?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.APIKey{}, &model.AdminAuditLog{}))

	store := sqlstore.NewWithDB(db)
	svc := NewService(store, nil, "secret", time.Hour, "cookie", fakeCodec{}, "salt", nil, nil, nil)

	user := &model.User{
		GoogleSub:  model.StringPtr("user-sub"),
		Email:      "demo@example.com",
		Name:       "Demo",
		InviteCode: "invite-000001",
		Status:     model.UserStatusActive,
	}
	require.NoError(t, db.Create(user).Error)

	otherUserID := uint64(999)
	activeQuota := 10
	keys := []model.APIKey{
		{OwnerUserID: &user.ID, KeyHash: "hash-1", KeyPrefix: "cop_live_1", Status: model.APIKeyStatusActive, PlanName: "Starter", QuotaTotal: &activeQuota},
		{OwnerUserID: &user.ID, KeyHash: "hash-2", KeyPrefix: "cop_live_2", Status: model.APIKeyStatusDisabled, PlanName: "Starter", QuotaTotal: &activeQuota},
		{OwnerUserID: &otherUserID, KeyHash: "hash-3", KeyPrefix: "cop_live_3", Status: model.APIKeyStatusActive, PlanName: "Starter", QuotaTotal: &activeQuota},
	}
	for i := range keys {
		require.NoError(t, db.Create(&keys[i]).Error)
	}

	status := string(model.UserStatusDisabled)
	require.NoError(t, svc.UpdateUser(context.Background(), user.ID, UpdateUserRequest{Status: &status}))

	savedUser, err := store.GetUserByID(context.Background(), user.ID)
	require.NoError(t, err)
	require.Equal(t, model.UserStatusDisabled, savedUser.Status)

	var owned []model.APIKey
	require.NoError(t, db.Where("owner_user_id = ?", user.ID).Order("id asc").Find(&owned).Error)
	require.Len(t, owned, 2)
	require.Equal(t, model.APIKeyStatusDisabled, owned[0].Status)
	require.Equal(t, model.APIKeyStatusDisabled, owned[1].Status)

	var other model.APIKey
	require.NoError(t, db.Where("key_hash = ?", "hash-3").First(&other).Error)
	require.Equal(t, model.APIKeyStatusActive, other.Status)
}

func TestUpdateUserReEnableDoesNotRestoreDisabledAPIKeys(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:update_user_reenable_does_not_restore_disabled_api_keys?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.APIKey{}, &model.AdminAuditLog{}))

	store := sqlstore.NewWithDB(db)
	svc := NewService(store, nil, "secret", time.Hour, "cookie", fakeCodec{}, "salt", nil, nil, nil)

	user := &model.User{
		GoogleSub:  model.StringPtr("user-sub"),
		Email:      "demo@example.com",
		Name:       "Demo",
		InviteCode: "invite-000001",
		Status:     model.UserStatusDisabled,
	}
	require.NoError(t, db.Create(user).Error)

	quota := 10
	key := &model.APIKey{
		OwnerUserID: &user.ID,
		KeyHash:     "hash-1",
		KeyPrefix:   "cop_live_1",
		Status:      model.APIKeyStatusDisabled,
		PlanName:    "Starter",
		QuotaTotal:  &quota,
	}
	require.NoError(t, db.Create(key).Error)

	status := string(model.UserStatusActive)
	require.NoError(t, svc.UpdateUser(context.Background(), user.ID, UpdateUserRequest{Status: &status}))

	savedUser, err := store.GetUserByID(context.Background(), user.ID)
	require.NoError(t, err)
	require.Equal(t, model.UserStatusActive, savedUser.Status)

	savedKey, err := store.FindAPIKeyByID(context.Background(), key.ID)
	require.NoError(t, err)
	require.Equal(t, model.APIKeyStatusDisabled, savedKey.Status)
}

func TestGetAPIKeyPlaintextRejectsLegacyRecords(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.APIKey{}, &model.AdminAuditLog{}))
	store := sqlstore.NewWithDB(db)
	cipher, err := apikey.NewCipher(apikey.DefaultDevEncryptionKey)
	require.NoError(t, err)
	svc := NewService(store, redisstore.NewStore(redis.NewClient(&redis.Options{Addr: "localhost:6379"})), "secret", time.Hour, "cookie", fakeCodec{}, "salt", cipher, nil, nil)

	key := &model.APIKey{KeyHash: "legacy-hash", KeyPrefix: "cop_legacy", Status: model.APIKeyStatusActive, PlanName: "Legacy"}
	require.NoError(t, store.CreateAPIKey(context.Background(), key))

	_, err = svc.GetAPIKeyPlaintext(context.Background(), key.ID, "admin@example.com")
	require.ErrorIs(t, err, apikey.ErrPlaintextUnavailable)
}
