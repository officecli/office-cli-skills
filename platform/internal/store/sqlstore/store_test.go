package sqlstore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	growthsvc "github.com/officecli/officecli/platform/internal/growth"
	"github.com/officecli/officecli/platform/internal/model"
)

func strPtr(value string) *string { return &value }

func timePtr(value time.Time) *time.Time { return &value }

type queryCaptureLogger struct {
	logger.Interface
	statements []string
}

func (l *queryCaptureLogger) LogMode(level logger.LogLevel) logger.Interface {
	return l
}

func (l *queryCaptureLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	l.statements = append(l.statements, sql)
}

func TestPostgresMigrationVersionsAreUnique(t *testing.T) {
	migrationPaths, err := filepath.Glob(filepath.Join("..", "..", "..", "migrations", "postgres", "*.sql"))
	require.NoError(t, err)
	require.NotEmpty(t, migrationPaths)

	seen := map[string]string{}
	for _, migrationPath := range migrationPaths {
		version := migrationVersion(migrationPath)
		if previous, ok := seen[version]; ok {
			t.Fatalf("duplicate migration version %s: %s and %s", version, filepath.Base(previous), filepath.Base(migrationPath))
		}
		seen[version] = migrationPath
	}
}

func TestUserHostedCreditLedgerMigrationsIncludeRuntimeColumns(t *testing.T) {
	requiredColumns := []string{"usage_event_id", "order_id"}
	migrationPaths := []string{
		filepath.Join("..", "..", "..", "migrations", "016_user_hosted_credit_accounts.sql"),
		filepath.Join("..", "..", "..", "migrations", "postgres", "016_user_hosted_credit_accounts.sql"),
		filepath.Join("..", "..", "..", "migrations", "postgres", "017_user_hosted_credit_ledger_runtime_columns.sql"),
	}

	for _, migrationPath := range migrationPaths {
		t.Run(migrationPath, func(t *testing.T) {
			sqlBytes, err := os.ReadFile(migrationPath)
			require.NoError(t, err)
			sql := strings.ToLower(string(sqlBytes))
			for _, column := range requiredColumns {
				require.Contains(t, sql, column)
			}
		})
	}
}

func TestUsageEventAuditFieldsAndFilters(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:usage_event_audit_filters?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.UsageEvent{}))
	store := NewWithDB(db)

	requestID := "req-audit-1"
	clientIP := "203.0.113.42"
	forwardedFor := "198.51.100.7, 10.0.0.4"
	userAgent := "officecli/0.2.65 audit-test"
	requestHost := "platform.officecli.io"
	requestPath := "/api/license/check"
	requestMethod := "POST"
	userID := uint64(42)
	apiKeyID := uint64(7)
	createdAt := time.Date(2026, 5, 15, 8, 0, 0, 0, time.UTC)
	event := &model.UsageEvent{
		RequestID:       &requestID,
		FingerprintHash: "fp-audit",
		Mode:            model.UsageModePaid,
		Action:          model.UsageActionGenerate,
		APIKeyID:        &apiKeyID,
		UserID:          &userID,
		Result:          model.UsageResultAllowed,
		ClientIP:        &clientIP,
		ForwardedFor:    &forwardedFor,
		UserAgent:       &userAgent,
		RequestHost:     &requestHost,
		RequestPath:     &requestPath,
		RequestMethod:   &requestMethod,
		CreatedAt:       createdAt,
	}
	require.NoError(t, store.CreateUsageEvent(context.Background(), event))

	start := createdAt.Add(-time.Minute)
	end := createdAt.Add(time.Minute)
	events, err := store.ListUsageEvents(context.Background(), UsageEventFilter{
		APIKeyID:  &apiKeyID,
		UserID:    &userID,
		ClientIP:  "203.0.113",
		RequestID: "req-audit",
		StartTime: &start,
		EndTime:   &end,
	})
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "fp-audit", events[0].FingerprintHash)
	require.NotNil(t, events[0].ClientIP)
	require.Equal(t, clientIP, *events[0].ClientIP)
	require.NotNil(t, events[0].ForwardedFor)
	require.Equal(t, forwardedFor, *events[0].ForwardedFor)
	require.NotNil(t, events[0].UserAgent)
	require.Equal(t, userAgent, *events[0].UserAgent)
	require.NotNil(t, events[0].RequestHost)
	require.Equal(t, requestHost, *events[0].RequestHost)
	require.NotNil(t, events[0].RequestPath)
	require.Equal(t, requestPath, *events[0].RequestPath)
	require.NotNil(t, events[0].RequestMethod)
	require.Equal(t, requestMethod, *events[0].RequestMethod)

	events, err = store.ListUsageEvents(context.Background(), UsageEventFilter{ClientIP: "192.0.2"})
	require.NoError(t, err)
	require.Empty(t, events)
}

func TestListUsageEventsFiltersByMultiModeResultAndAction(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:usage_multi_filters?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.UsageEvent{}))
	store := NewWithDB(db)

	baseTime := time.Date(2026, 5, 27, 8, 0, 0, 0, time.UTC)
	events := []model.UsageEvent{
		{FingerprintHash: "fp-free-generate", Mode: model.UsageModeFree, Action: model.UsageActionGenerate, Result: model.UsageResultAllowed, CreatedAt: baseTime.Add(3 * time.Minute)},
		{FingerprintHash: "fp-reward-generate", Mode: model.UsageModeReward, Action: model.UsageActionGenerate, Result: model.UsageResultBlocked, CreatedAt: baseTime.Add(2 * time.Minute)},
		{FingerprintHash: "fp-hosted-status", Mode: model.UsageModeHosted, Action: model.UsageActionStatus, Result: model.UsageResultAllowed, CreatedAt: baseTime.Add(time.Minute)},
		{FingerprintHash: "fp-paid-generate", Mode: model.UsageModePaid, Action: model.UsageActionGenerate, Result: model.UsageResultAllowed, CreatedAt: baseTime},
	}
	for i := range events {
		require.NoError(t, store.CreateUsageEvent(context.Background(), &events[i]))
	}

	matched, err := store.ListUsageEvents(context.Background(), UsageEventFilter{
		Modes:   []string{"free", "reward", "hosted"},
		Results: []string{"allowed", "blocked"},
		Actions: []string{"generate"},
	})
	require.NoError(t, err)
	require.Len(t, matched, 2)
	require.Equal(t, "fp-free-generate", matched[0].FingerprintHash)
	require.Equal(t, "fp-reward-generate", matched[1].FingerprintHash)
}

func TestFingerprintQualityClassifiesAndAggregatesUsageEvents(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:fingerprint_quality?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.UsageEvent{}))
	store := NewWithDB(db)

	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	userID := uint64(42)
	inspectionUA := "officecli-production-inspection/1.0"
	dev := " dev "
	current := "current-20260520"
	hosted := " hosted "
	docx := "docx"
	xlsx := " xlsx "
	pptx := "pptx"
	normalVersion := "0.2.65"
	localSmoke := "0.2.65-local-smoke"
	candidateHash := strings.Repeat("a", 64)
	matrixHash := strings.Repeat("b", 64)
	currentHash := strings.Repeat("c", 64)
	devHash := strings.Repeat("d", 64)
	inspectionHash := strings.Repeat("e", 64)

	require.NoError(t, db.Create(&[]model.UsageEvent{
		{FingerprintHash: inspectionHash, Mode: model.UsageModeFree, Action: model.UsageActionGenerate, Result: model.UsageResultAllowed, UserAgent: &inspectionUA, CreatedAt: now},
		{FingerprintHash: "", Mode: model.UsageModeFree, Action: model.UsageActionStatus, Result: model.UsageResultBlocked, CreatedAt: now.Add(time.Minute)},
		{FingerprintHash: "not-a-sha256", Mode: model.UsageModeFree, Action: model.UsageActionGenerate, Result: model.UsageResultAllowed, CreatedAt: now.Add(2 * time.Minute)},
		{FingerprintHash: currentHash, Mode: model.UsageModeFree, Action: model.UsageActionGenerate, Result: model.UsageResultAllowed, CLIVersion: &current, CreatedAt: now.Add(3 * time.Minute)},
		{FingerprintHash: matrixHash, Mode: model.UsageModeHosted, Action: model.UsageActionGenerate, Result: model.UsageResultAllowed, CLIVersion: &dev, RuntimeMode: &hosted, DocumentType: &xlsx, ClientIP: strPtr(" 10.0.0.2 "), UserID: &userID, CreatedAt: now.Add(4 * time.Minute)},
		{FingerprintHash: matrixHash, Mode: model.UsageModeHosted, Action: model.UsageActionStatus, Result: model.UsageResultBlocked, CLIVersion: &dev, RuntimeMode: &hosted, DocumentType: &docx, ClientIP: strPtr("10.0.0.1"), UserID: &userID, CreatedAt: now.Add(5 * time.Minute)},
		{FingerprintHash: matrixHash, Mode: model.UsageModeHosted, Action: model.UsageActionGenerate, Result: model.UsageResultAllowed, CLIVersion: &dev, RuntimeMode: &hosted, DocumentType: &pptx, ClientIP: strPtr("10.0.0.1"), CreatedAt: now.Add(6 * time.Minute)},
		{FingerprintHash: devHash, Mode: model.UsageModeFree, Action: model.UsageActionGenerate, Result: model.UsageResultAllowed, CLIVersion: &localSmoke, CreatedAt: now.Add(7 * time.Minute)},
		{FingerprintHash: candidateHash, Mode: model.UsageModePaid, Action: model.UsageActionGenerate, Result: model.UsageResultAllowed, CLIVersion: &normalVersion, UserAgent: strPtr("Go-http-client/2.0"), CreatedAt: now.Add(8 * time.Minute)},
	}).Error)

	quality, err := store.FingerprintQuality(context.Background())
	require.NoError(t, err)
	require.Len(t, quality.Rows, 7)

	byHash := map[string]model.FingerprintQualityRow{}
	for _, row := range quality.Rows {
		byHash[row.FingerprintHash] = row
	}

	require.Equal(t, "internal_inspection", byHash[inspectionHash].Bucket)
	require.Equal(t, "test_fake_fingerprint", byHash[""].Bucket)
	require.Equal(t, "test_fake_fingerprint", byHash["not-a-sha256"].Bucket)
	require.Equal(t, "likely_docker_ci_current_build", byHash[currentHash].Bucket)
	require.Equal(t, "likely_docker_dev_hosted_matrix", byHash[matrixHash].Bucket)
	require.Equal(t, "likely_internal_test_dev_latest_local", byHash[devHash].Bucket)
	require.Equal(t, "candidate_real_or_unknown", byHash[candidateHash].Bucket)

	matrix := byHash[matrixHash]
	require.EqualValues(t, 3, matrix.Events)
	require.EqualValues(t, 2, matrix.GenerateEvents)
	require.EqualValues(t, 1, matrix.StatusEvents)
	require.EqualValues(t, 1, matrix.BlockedEvents)
	require.EqualValues(t, 2, matrix.UserBoundEvents)
	require.EqualValues(t, 2, matrix.IPCount)
	require.Equal(t, []string{"10.0.0.1", "10.0.0.2"}, matrix.IPs)
	require.Equal(t, []string{"dev"}, matrix.CLIVersions)
	require.Equal(t, []string{"hosted"}, matrix.RuntimeModes)
	require.Equal(t, []string{"docx", "pptx", "xlsx"}, matrix.DocumentTypes)
	require.Equal(t, now.Add(4*time.Minute), matrix.FirstAt)
	require.Equal(t, now.Add(6*time.Minute), matrix.LastAt)

	summary := map[string]model.FingerprintQualityBucketSummary{}
	for _, item := range quality.Summary {
		summary[item.Bucket] = item
	}
	require.EqualValues(t, 1, summary["internal_inspection"].Fingerprints)
	require.EqualValues(t, 1, summary["internal_inspection"].Events)
	require.True(t, summary["internal_inspection"].DefaultFiltered)
	require.EqualValues(t, 2, summary["test_fake_fingerprint"].Fingerprints)
	require.EqualValues(t, 2, summary["test_fake_fingerprint"].Events)
	require.EqualValues(t, 1, summary["candidate_real_or_unknown"].Fingerprints)
	require.EqualValues(t, 1, summary["candidate_real_or_unknown"].Events)
	require.False(t, summary["candidate_real_or_unknown"].DefaultFiltered)
}

func TestFingerprintQualityUsesFingerprintLevelAggregationAndCapsDistinctValues(t *testing.T) {
	capture := &queryCaptureLogger{Interface: logger.Default.LogMode(logger.Info)}
	db, err := gorm.Open(sqlite.Open("file:fingerprint_quality_grouped?mode=memory&cache=shared"), &gorm.Config{Logger: capture})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.UsageEvent{}))
	store := NewWithDB(db)

	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	fingerprint := strings.Repeat("a", 64)
	otherFingerprint := strings.Repeat("b", 64)
	events := make([]model.UsageEvent, 0, fingerprintQualityDistinctValueLimit+15)
	for i := 0; i < fingerprintQualityDistinctValueLimit+15; i++ {
		ip := fmt.Sprintf("10.0.0.%02d", i)
		cliVersion := fmt.Sprintf("0.2.%02d", i)
		runtimeMode := fmt.Sprintf("runtime-%02d", i)
		documentType := fmt.Sprintf("doc-%02d", i)
		userAgent := fmt.Sprintf("agent-%02d", i)
		events = append(events, model.UsageEvent{
			FingerprintHash: fingerprint,
			Mode:            model.UsageModePaid,
			Action:          model.UsageActionGenerate,
			Result:          model.UsageResultAllowed,
			ClientIP:        &ip,
			CLIVersion:      &cliVersion,
			RuntimeMode:     &runtimeMode,
			DocumentType:    &documentType,
			UserAgent:       &userAgent,
			CreatedAt:       now.Add(time.Duration(i) * time.Minute),
		})
	}
	events = append(events, model.UsageEvent{
		FingerprintHash: otherFingerprint,
		Mode:            model.UsageModeFree,
		Action:          model.UsageActionStatus,
		Result:          model.UsageResultBlocked,
		CreatedAt:       now.Add(2 * time.Hour),
	})
	require.NoError(t, db.Create(&events).Error)

	capture.statements = nil
	quality, err := store.FingerprintQuality(context.Background())
	require.NoError(t, err)
	require.Len(t, quality.Rows, 2)
	require.Len(t, quality.Summary, 6)

	byHash := map[string]model.FingerprintQualityRow{}
	for _, row := range quality.Rows {
		byHash[row.FingerprintHash] = row
	}
	row := byHash[fingerprint]
	require.EqualValues(t, fingerprintQualityDistinctValueLimit+15, row.Events)
	require.EqualValues(t, fingerprintQualityDistinctValueLimit+15, row.IPCount)
	require.Equal(t, now, row.FirstAt)
	require.Equal(t, now.Add(time.Duration(fingerprintQualityDistinctValueLimit+14)*time.Minute), row.LastAt)
	require.Len(t, row.IPs, fingerprintQualityDistinctValueLimit)
	require.Len(t, row.CLIVersions, fingerprintQualityDistinctValueLimit)
	require.Len(t, row.RuntimeModes, fingerprintQualityDistinctValueLimit)
	require.Len(t, row.DocumentTypes, fingerprintQualityDistinctValueLimit)
	require.Len(t, row.UserAgents, fingerprintQualityDistinctValueLimit)

	sqlLog := strings.ToLower(strings.Join(capture.statements, "\n"))
	require.Contains(t, sqlLog, "group by")
	require.Contains(t, sqlLog, "fingerprint_hash")
	require.NotContains(t, sqlLog, "select fingerprint_hash, created_at, action, result, user_id, client_ip, cli_version, runtime_mode, document_type, user_agent")
}

func TestClassifyFingerprintQualityPriorityAndVersionRules(t *testing.T) {
	valid := strings.Repeat("f", 64)
	cases := []struct {
		name   string
		row    model.FingerprintQualityRow
		bucket string
	}{
		{
			name: "inspection wins over invalid fingerprint",
			row: model.FingerprintQualityRow{
				FingerprintHash: "invalid",
				UserAgents:      []string{" officecli-production-inspection/1.0 "},
				CLIVersions:     []string{"current-20260520"},
			},
			bucket: "internal_inspection",
		},
		{
			name: "current build wins over docker dev hosted matrix",
			row: model.FingerprintQualityRow{
				FingerprintHash: valid,
				CLIVersions:     []string{"dev", "current-20260520"},
				RuntimeModes:    []string{"hosted"},
				DocumentTypes:   []string{"docx", "xlsx"},
			},
			bucket: "likely_docker_ci_current_build",
		},
		{
			name:   "exact dev matches internal test bucket",
			row:    model.FingerprintQualityRow{FingerprintHash: valid, CLIVersions: []string{"dev"}},
			bucket: "likely_internal_test_dev_latest_local",
		},
		{
			name:   "latest prefix matches internal test bucket",
			row:    model.FingerprintQualityRow{FingerprintHash: valid, CLIVersions: []string{"latest-20260520"}},
			bucket: "likely_internal_test_dev_latest_local",
		},
		{
			name:   "local prefix matches internal test bucket",
			row:    model.FingerprintQualityRow{FingerprintHash: valid, CLIVersions: []string{"local-smoke"}},
			bucket: "likely_internal_test_dev_latest_local",
		},
		{
			name:   "local smoke suffix matches internal test bucket",
			row:    model.FingerprintQualityRow{FingerprintHash: valid, CLIVersions: []string{"0.2.65-local-smoke"}},
			bucket: "likely_internal_test_dev_latest_local",
		},
		{
			name:   "dev is exact and does not match preview-devops",
			row:    model.FingerprintQualityRow{FingerprintHash: valid, CLIVersions: []string{"preview-devops"}},
			bucket: "candidate_real_or_unknown",
		},
		{
			name:   "dev is exact and does not match nodev",
			row:    model.FingerprintQualityRow{FingerprintHash: valid, CLIVersions: []string{"nodev"}},
			bucket: "candidate_real_or_unknown",
		},
		{
			name:   "go http client alone does not match test buckets",
			row:    model.FingerprintQualityRow{FingerprintHash: valid, UserAgents: []string{"Go-http-client/2.0"}},
			bucket: "candidate_real_or_unknown",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bucket, _ := classifyFingerprintQuality(tc.row)
			require.Equal(t, tc.bucket, bucket)
		})
	}
}

func TestOperationsFunnelUsesWindowedOperationalAndFactTables(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:operations_funnel?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.OperationalEvent{},
		&model.User{},
		&model.CLILoginChallenge{},
		&model.CLISession{},
		&model.UsageEvent{},
		&model.Order{},
	))
	store := NewWithDB(db)

	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	windowStart := now.AddDate(0, 0, -30)
	outside := windowStart.Add(-time.Hour)
	recent := now.Add(-2 * time.Hour)
	user1 := uint64(1)
	user2 := uint64(2)
	user4 := uint64(4)

	require.NoError(t, db.Create(&[]model.OperationalEvent{
		{EventName: "pricing_view", Surface: "site", VisitorID: strPtr("visitor-1"), MetadataJSON: `{}`, CreatedAt: recent},
		{EventName: "download_click", Surface: "site", VisitorID: strPtr("visitor-2"), MetadataJSON: `{}`, CreatedAt: recent},
		{EventName: "console_open", Surface: "site", VisitorID: strPtr("visitor-2"), MetadataJSON: `{}`, CreatedAt: recent},
		{EventName: "cta_click", Surface: "app", VisitorID: strPtr("visitor-3"), MetadataJSON: `{}`, CreatedAt: recent},
		{EventName: "login_start", Surface: "app", VisitorID: strPtr("visitor-3"), MetadataJSON: `{}`, CreatedAt: recent},
		{EventName: "checkout_start", Surface: "app", VisitorID: strPtr("visitor-3"), MetadataJSON: `{}`, CreatedAt: recent},
		{EventName: "pricing_view", Surface: "site", VisitorID: strPtr("visitor-old"), MetadataJSON: `{}`, CreatedAt: outside},
	}).Error)
	require.NoError(t, db.Create(&[]model.User{
		{ID: 1, GoogleSub: model.StringPtr("sub-1"), Email: "u1@example.com", Name: "U1", InviteCode: "invite-1", Status: model.UserStatusActive, CreatedAt: recent},
		{ID: 2, GoogleSub: model.StringPtr("sub-2"), Email: "u2@example.com", Name: "U2", InviteCode: "invite-2", Status: model.UserStatusActive, CreatedAt: recent},
		{ID: 3, GoogleSub: model.StringPtr("sub-3"), Email: "u3@example.com", Name: "U3", InviteCode: "invite-3", Status: model.UserStatusActive, CreatedAt: outside},
		{ID: 4, GoogleSub: model.StringPtr("sub-4"), Email: "u4@example.com", Name: "U4", InviteCode: "invite-4", Status: model.UserStatusActive, CreatedAt: outside},
	}).Error)
	require.NoError(t, db.Create(&[]model.CLILoginChallenge{
		{ChallengeID: "challenge-1", Flow: model.CLILoginChallengeFlowCallback, CodeChallenge: "cc", CodeChallengeMethod: "plain", RedirectURI: "http://localhost", State: "s1", Status: model.CLILoginChallengeStatusCompleted, ExpiresAt: now, CompletedAt: timePtr(recent), CreatedAt: recent},
		{ChallengeID: "challenge-2", Flow: model.CLILoginChallengeFlowCallback, CodeChallenge: "cc", CodeChallengeMethod: "plain", RedirectURI: "http://localhost", State: "s2", Status: model.CLILoginChallengeStatusPending, ExpiresAt: now, CreatedAt: recent},
		{ChallengeID: "challenge-old", Flow: model.CLILoginChallengeFlowCallback, CodeChallenge: "cc", CodeChallengeMethod: "plain", RedirectURI: "http://localhost", State: "s3", Status: model.CLILoginChallengeStatusCompleted, ExpiresAt: now, CompletedAt: timePtr(outside), CreatedAt: outside},
	}).Error)
	require.NoError(t, db.Create(&[]model.CLISession{
		{UserID: 1, TokenHash: "token-1", TokenPrefix: "tok1", Name: "laptop", ExpiresAt: now, CreatedAt: recent},
		{UserID: 3, TokenHash: "token-old", TokenPrefix: "tok2", Name: "old", ExpiresAt: now, CreatedAt: outside},
	}).Error)
	require.NoError(t, db.Create(&[]model.UsageEvent{
		{FingerprintHash: "machine-u1", UserID: &user1, Mode: model.UsageModeFree, Action: model.UsageActionGenerate, Result: model.UsageResultAllowed, CreatedAt: recent},
		{FingerprintHash: "machine-u1", UserID: &user1, Mode: model.UsageModeFree, Action: model.UsageActionGenerate, Result: model.UsageResultBlocked, CreatedAt: recent.Add(time.Minute)},
		{FingerprintHash: "machine-u2", UserID: &user2, Mode: model.UsageModePaid, Action: model.UsageActionGenerate, Result: model.UsageResultAllowed, CreatedAt: outside},
		{FingerprintHash: "machine-u2", UserID: &user2, Mode: model.UsageModePaid, Action: model.UsageActionGenerate, Result: model.UsageResultAllowed, CreatedAt: recent},
		{FingerprintHash: "machine-u4", UserID: &user4, Mode: model.UsageModePaid, Action: model.UsageActionGenerate, Result: model.UsageResultAllowed, CreatedAt: recent},
		{FingerprintHash: "machine-anon", Mode: model.UsageModeFree, Action: model.UsageActionGenerate, Result: model.UsageResultAllowed, CreatedAt: recent},
		{FingerprintHash: "machine-anon", Mode: model.UsageModeFree, Action: model.UsageActionGenerate, Result: model.UsageResultAllowed, CreatedAt: recent.Add(time.Minute)},
		{FingerprintHash: "hosted-fp", Mode: model.UsageModeHosted, Action: model.UsageActionGenerate, Result: model.UsageResultAllowed, CreatedAt: recent},
	}).Error)
	require.NoError(t, db.Create(&[]model.Order{
		{UserID: 1, Status: model.OrderStatusPaid, Currency: "usd", AmountTotal: 1000, PackCode: "p1", PackName: "P1", QuotaAmount: 100, CreatedAt: recent, UpdatedAt: recent},
		{UserID: 1, Status: model.OrderStatusPaid, Currency: "usd", AmountTotal: 2000, PackCode: "p2", PackName: "P2", QuotaAmount: 200, CreatedAt: outside, UpdatedAt: recent},
		{UserID: 2, Status: model.OrderStatusPending, Currency: "usd", AmountTotal: 3000, PackCode: "p3", PackName: "P3", QuotaAmount: 300, CreatedAt: recent, UpdatedAt: recent},
		{UserID: 3, Status: model.OrderStatusPaid, Currency: "usd", AmountTotal: 4000, PackCode: "p4", PackName: "P4", QuotaAmount: 400, CreatedAt: outside, UpdatedAt: outside},
		{UserID: 4, Status: model.OrderStatusPaid, Currency: "usd", AmountTotal: 5000, PackCode: "p5", PackName: "P5", QuotaAmount: 500, CreatedAt: outside, UpdatedAt: recent},
	}).Error)

	funnel, err := store.OperationsFunnel(context.Background(), windowStart, now)
	require.NoError(t, err)
	require.EqualValues(t, 3, funnel.Visitors)
	require.EqualValues(t, 1, funnel.PricingViews)
	require.EqualValues(t, 3, funnel.CTAClicks)
	require.EqualValues(t, 1, funnel.LoginStarts)
	require.EqualValues(t, 2, funnel.RegisteredUsers)
	require.EqualValues(t, 3, funnel.ActivatedUsers)
	require.EqualValues(t, 2, funnel.ActivatedRegisteredUsers)
	require.Equal(t, 1.0, funnel.ActivationRate)
	require.EqualValues(t, 2, funnel.CLILoginStarted)
	require.EqualValues(t, 1, funnel.CLILoginCompleted)
	require.EqualValues(t, 1, funnel.CLISessionsCreated)
	require.EqualValues(t, 3, funnel.FirstUsageUsers)
	require.EqualValues(t, 2, funnel.RepeatUsageUsers)
	require.InDelta(t, 1.0/7.0, funnel.BlockedRate, 0.0001)
	require.EqualValues(t, 1, funnel.CheckoutStarts)
	require.EqualValues(t, 2, funnel.OrdersCreated)
	require.EqualValues(t, 3, funnel.PaidOrders)
	require.EqualValues(t, 2, funnel.PaidUsers)
	require.EqualValues(t, 1, funnel.PaidRegisteredUsers)
	require.EqualValues(t, 8000, funnel.Revenue)
	require.Equal(t, 0.5, funnel.PaidConversionRate)
	require.Equal(t, 3.0, funnel.CheckoutToPaidRate)
	require.EqualValues(t, 1, funnel.RepeatPaidUsers)
	require.EqualValues(t, 4, funnel.MachineQuality.TotalMachines)
	require.EqualValues(t, 4, funnel.MachineQuality.ActiveMachines24h)
	require.EqualValues(t, 4, funnel.MachineQuality.ActiveMachines7d)
	require.EqualValues(t, funnel.FirstUsageUsers, funnel.UsageQuality.FirstUsageUsers)
	require.EqualValues(t, funnel.Revenue, funnel.RevenueQuality.Revenue)
}

func TestOperationsFunnelRatesAreZeroWithoutRegisteredUsers(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:operations_funnel_zero_registered?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.CLISession{}, &model.UsageEvent{}, &model.Order{}, &model.OperationalEvent{}, &model.CLILoginChallenge{}))
	store := NewWithDB(db)

	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	windowStart := now.AddDate(0, 0, -30)
	outside := windowStart.Add(-time.Hour)
	recent := now.Add(-time.Hour)
	oldUserID := uint64(10)
	require.NoError(t, db.Create(&model.User{ID: oldUserID, GoogleSub: model.StringPtr("old-sub"), Email: "old@example.com", Name: "Old", InviteCode: "invite-old", Status: model.UserStatusActive, CreatedAt: outside}).Error)
	require.NoError(t, db.Create(&model.UsageEvent{FingerprintHash: "old-machine", UserID: &oldUserID, Mode: model.UsageModePaid, Action: model.UsageActionGenerate, Result: model.UsageResultAllowed, CreatedAt: recent}).Error)
	require.NoError(t, db.Create(&model.Order{UserID: oldUserID, Status: model.OrderStatusPaid, Currency: "usd", AmountTotal: 1000, PackCode: "old", PackName: "Old", QuotaAmount: 100, CreatedAt: outside, UpdatedAt: recent}).Error)

	funnel, err := store.OperationsFunnel(context.Background(), windowStart, now)
	require.NoError(t, err)
	require.EqualValues(t, 0, funnel.RegisteredUsers)
	require.EqualValues(t, 1, funnel.ActivatedUsers)
	require.EqualValues(t, 0, funnel.ActivatedRegisteredUsers)
	require.Equal(t, 0.0, funnel.ActivationRate)
	require.EqualValues(t, 1, funnel.PaidUsers)
	require.EqualValues(t, 0, funnel.PaidRegisteredUsers)
	require.Equal(t, 0.0, funnel.PaidConversionRate)
}

func TestOverviewIncludesChartData(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:overview_chart_data?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.APIKey{}, &model.OperationalEvent{}, &model.UsageEvent{}, &model.FingerprintCreditAccount{}, &model.FingerprintCreditLedger{}, &model.User{}, &model.Order{}, &model.CLILoginChallenge{}, &model.CLISession{}))
	store := NewWithDB(db)

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC)
	yesterday := today.AddDate(0, 0, -1)
	threeDaysAgo := today.AddDate(0, 0, -3)
	outsideWindow := today.AddDate(0, 0, -8)

	expiredAt := now.Add(-time.Hour)
	futureExpiry := now.Add(24 * time.Hour)
	require.NoError(t, db.Create(&[]model.APIKey{
		{KeyHash: "active-hash", KeyPrefix: "active", Status: model.APIKeyStatusActive, PlanName: "Active", QuotaUsed: 0},
		{KeyHash: "disabled-hash", KeyPrefix: "disabled", Status: model.APIKeyStatusDisabled, PlanName: "Disabled", QuotaUsed: 0, ExpiresAt: &futureExpiry},
		{KeyHash: "expired-hash", KeyPrefix: "expired", Status: model.APIKeyStatusActive, PlanName: "Expired", QuotaUsed: 0, ExpiresAt: &expiredAt},
		{KeyHash: "other-hash", KeyPrefix: "other", Status: model.APIKeyStatus("pending"), PlanName: "Other", QuotaUsed: 0},
	}).Error)

	events := []model.UsageEvent{
		{FingerprintHash: "fp-allowed-check", Mode: model.UsageModeFree, Action: model.UsageActionStatus, Result: model.UsageResultAllowed, Charged: false, CreatedAt: today},
		{FingerprintHash: "fp-hosted-consume", Mode: model.UsageModeHosted, Action: model.UsageActionGenerate, Result: model.UsageResultAllowed, Charged: true, CreatedAt: today.Add(time.Hour)},
		{FingerprintHash: "fp-blocked", Mode: model.UsageModeReward, Action: model.UsageActionGenerate, Result: model.UsageResultBlocked, Charged: false, CreatedAt: yesterday},
		{FingerprintHash: "fp-paid", Mode: model.UsageModePaid, Action: model.UsageActionGenerate, Result: model.UsageResultAllowed, Charged: true, CreatedAt: threeDaysAgo},
		{FingerprintHash: "fp-old", Mode: model.UsageModePaid, Action: model.UsageActionGenerate, Result: model.UsageResultBlocked, Charged: true, CreatedAt: outsideWindow},
	}
	require.NoError(t, db.Create(&events).Error)
	require.NoError(t, db.Create(&[]model.FingerprintCreditAccount{
		{FingerprintHash: "fp-credit-1", CreditBalance: 90, BonusGranted: true},
		{FingerprintHash: "fp-credit-2", CreditBalance: 50, BonusGranted: true},
	}).Error)

	overview, err := store.Overview(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 2, overview.FreeMachines)
	require.Len(t, overview.UsageTrend, 7)
	require.Equal(t, today.Format("2006-01-02"), overview.UsageTrend[6].Date)
	require.EqualValues(t, 2, overview.UsageTrend[6].Total)
	require.EqualValues(t, 1, overview.UsageTrend[6].Checks)
	require.EqualValues(t, 1, overview.UsageTrend[6].Consumes)
	require.EqualValues(t, 0, overview.UsageTrend[6].Blocked)
	require.EqualValues(t, 2, overview.UsageTrend[6].Allowed)
	require.Equal(t, threeDaysAgo.Format("2006-01-02"), overview.UsageTrend[3].Date)
	require.EqualValues(t, 1, overview.UsageTrend[3].Consumes)

	require.Equal(t, []model.OverviewBreakdownItem{
		{Key: "free", Label: "Free", Value: 1},
		{Key: "reward", Label: "Reward", Value: 1},
		{Key: "paid", Label: "Paid", Value: 1},
		{Key: "hosted", Label: "Hosted", Value: 1},
	}, overview.ModeBreakdown)
	require.Equal(t, []model.OverviewBreakdownItem{
		{Key: "allowed", Label: "Allowed", Value: 3},
		{Key: "blocked", Label: "Blocked", Value: 1},
	}, overview.ResultBreakdown)
	require.Equal(t, []model.OverviewBreakdownItem{
		{Key: "active", Label: "Active", Value: 1},
		{Key: "disabled", Label: "Disabled", Value: 1},
		{Key: "expired", Label: "Expired", Value: 1},
		{Key: "other", Label: "Other", Value: 1},
	}, overview.APIKeyStatusBreakdown)
	require.NotNil(t, overview.OperationsFunnel30d)
}

func TestAdminUserPreferenceUpsertGetAndIsolation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:admin_user_preferences?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.AdminUserPreference{}))
	store := NewWithDB(db)

	ctx := context.Background()
	first, err := store.UpsertAdminUserPreference(ctx, "Ops@Example.com ", "usage-events-table", `{"version":1,"columns":[{"key":"created_at","visible":true}]}`)
	require.NoError(t, err)
	require.Equal(t, "ops@example.com", first.AdminEmail)
	require.Equal(t, "usage-events-table", first.PageKey)

	found, err := store.GetAdminUserPreference(ctx, "ops@example.com", "usage-events-table")
	require.NoError(t, err)
	require.NotNil(t, found)
	require.JSONEq(t, `{"version":1,"columns":[{"key":"created_at","visible":true}]}`, found.PreferencesJSON)

	updated, err := store.UpsertAdminUserPreference(ctx, "ops@example.com", "usage-events-table", `{"version":1,"columns":[{"key":"mode_result","visible":false,"fixed":"left"}]}`)
	require.NoError(t, err)
	require.Equal(t, first.ID, updated.ID)
	require.JSONEq(t, `{"version":1,"columns":[{"key":"mode_result","visible":false,"fixed":"left"}]}`, updated.PreferencesJSON)

	other, err := store.GetAdminUserPreference(ctx, "other@example.com", "usage-events-table")
	require.NoError(t, err)
	require.Nil(t, other)
}

func TestGrantExistingUsers100CreditBonusMigrationShape(t *testing.T) {
	migrationPath := filepath.Join("..", "..", "..", "migrations", "postgres", "025_grant_existing_users_100_credit_bonus.sql")
	sqlBytes, err := os.ReadFile(migrationPath)
	require.NoError(t, err)
	lower := strings.ToLower(string(sqlBytes))

	requiredSubstrings := []string{
		"signup-bonus-bump-100:",
		"'migration'",
		"on conflict (idempotency_key) do nothing",
		"user_hosted_credit_accounts",
		"user_hosted_credit_ledger",
		"from users",
		"left join user_hosted_credit_ledger",
		"is null",
		"credit_delta",
		"signup_bonus_bump_100",
	}
	for _, needle := range requiredSubstrings {
		require.Containsf(t, lower, needle, "025 migration must contain %q", needle)
	}
}

func TestBackfillSignupHostedCreditsMigrationShape(t *testing.T) {
	migrationPath := filepath.Join("..", "..", "..", "migrations", "postgres", "032_backfill_signup_hosted_credits.sql")
	sqlBytes, err := os.ReadFile(migrationPath)
	require.NoError(t, err)
	lower := strings.ToLower(string(sqlBytes))

	requiredSubstrings := []string{
		"signup-hosted-credits:",
		"'signup_bonus'",
		"on conflict (user_id) do update",
		"credit_balance = user_hosted_credit_accounts.credit_balance + excluded.credit_balance",
		"on conflict (idempotency_key) do nothing",
		"user_hosted_credit_accounts",
		"user_hosted_credit_ledger",
		"from users",
		"users.status = 'active'",
		"left join user_hosted_credit_ledger",
		"is null",
		"credit_delta",
		"backfill_signup_hosted_credits_20260528",
	}
	for _, needle := range requiredSubstrings {
		require.Containsf(t, lower, needle, "032 migration must contain %q", needle)
	}
}

func TestAssociateAnonymousUsageEventsMigrationShape(t *testing.T) {
	migrationPath := filepath.Join("..", "..", "..", "migrations", "postgres", "035_associate_anonymous_usage_events.sql")
	sqlBytes, err := os.ReadFile(migrationPath)
	require.NoError(t, err)
	lower := strings.ToLower(string(sqlBytes))

	requiredSubstrings := []string{
		"update usage_events",
		"from fingerprint_credit_accounts",
		"migrated_to_user_id",
		"usage_events.fingerprint_hash = fingerprint_credit_accounts.fingerprint_hash",
		"usage_events.user_id is null",
		"usage_events.user_id = 0",
	}
	for _, needle := range requiredSubstrings {
		require.Containsf(t, lower, needle, "035 migration must contain %q", needle)
	}
}

func TestAssociateAnonymousUsageEventsMigrationBackfillsMigratedFingerprints(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:associate_anonymous_usage_events_migration?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.FingerprintCreditAccount{}, &model.UsageEvent{}))
	store := NewWithDB(db)

	userID := uint64(89)
	otherUserID := uint64(90)
	fingerprint := "fp-migrated"
	require.NoError(t, db.Create(&model.FingerprintCreditAccount{
		FingerprintHash:  fingerprint,
		MigratedToUserID: &userID,
	}).Error)
	events := []model.UsageEvent{
		{FingerprintHash: fingerprint, Mode: model.UsageModeHosted, Action: model.UsageActionGenerate, Result: model.UsageResultAllowed},
		{FingerprintHash: fingerprint, UserID: &otherUserID, Mode: model.UsageModeHosted, Action: model.UsageActionGenerate, Result: model.UsageResultAllowed},
		{FingerprintHash: "fp-unmigrated", Mode: model.UsageModeHosted, Action: model.UsageActionGenerate, Result: model.UsageResultAllowed},
	}
	require.NoError(t, db.Create(&events).Error)

	migrationPath := filepath.Join("..", "..", "..", "migrations", "postgres", "035_associate_anonymous_usage_events.sql")
	require.NoError(t, store.ApplyMigrationFile(context.Background(), migrationPath))

	var migrated model.UsageEvent
	require.NoError(t, db.First(&migrated, events[0].ID).Error)
	require.NotNil(t, migrated.UserID)
	require.Equal(t, userID, *migrated.UserID)

	var alreadyOwned model.UsageEvent
	require.NoError(t, db.First(&alreadyOwned, events[1].ID).Error)
	require.NotNil(t, alreadyOwned.UserID)
	require.Equal(t, otherUserID, *alreadyOwned.UserID)

	var unrelated model.UsageEvent
	require.NoError(t, db.First(&unrelated, events[2].ID).Error)
	require.Nil(t, unrelated.UserID)
}

func TestCLILoginChallengeMigrationsIncludeDeviceFlowColumns(t *testing.T) {
	requiredColumns := []string{"flow", "user_code_hash"}
	migrationPaths := []string{
		filepath.Join("..", "..", "..", "migrations", "016_user_hosted_credit_accounts.sql"),
		filepath.Join("..", "..", "..", "migrations", "018_cli_login_device_flow.sql"),
		filepath.Join("..", "..", "..", "migrations", "postgres", "016_user_hosted_credit_accounts.sql"),
		filepath.Join("..", "..", "..", "migrations", "postgres", "018_cli_login_device_flow.sql"),
	}

	for _, migrationPath := range migrationPaths {
		t.Run(migrationPath, func(t *testing.T) {
			sqlBytes, err := os.ReadFile(migrationPath)
			require.NoError(t, err)
			sql := strings.ToLower(string(sqlBytes))
			for _, column := range requiredColumns {
				require.Contains(t, sql, column)
			}
		})
	}
}

func TestSaveGoogleUserGeneratesStableInviteCode(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:save_google_user_generates_stable_invite_code?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))

	store := NewWithDB(db)

	user, err := store.SaveGoogleUser(context.Background(), "google-sub-1", "demo@example.com", "Demo User", nil)
	require.NoError(t, err)
	require.Equal(t, "invite-000001", user.InviteCode)

	saved, err := store.GetUserByID(context.Background(), user.ID)
	require.NoError(t, err)
	require.Equal(t, "invite-000001", saved.InviteCode)

	updated, err := store.SaveGoogleUser(context.Background(), "google-sub-1", "demo@example.com", "Renamed User", nil)
	require.NoError(t, err)
	require.Equal(t, user.ID, updated.ID)
	require.Equal(t, "invite-000001", updated.InviteCode)
	require.Equal(t, "Renamed User", updated.Name)
}

func TestSaveGoogleUserBackfillsMissingInviteCode(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:save_google_user_backfills_missing_invite_code?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))

	legacy := &model.User{
		GoogleSub:  model.StringPtr("legacy-sub"),
		Email:      "legacy@example.com",
		Name:       "Legacy User",
		InviteCode: "",
		Status:     model.UserStatusActive,
	}
	require.NoError(t, db.Create(legacy).Error)

	store := NewWithDB(db)
	user, err := store.SaveGoogleUser(context.Background(), "legacy-sub", "legacy@example.com", "Legacy User", nil)
	require.NoError(t, err)
	require.Equal(t, buildInviteCode(legacy.ID), user.InviteCode)
}

func TestSaveGoogleUserKeepsDisabledStatusForExistingUser(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:save_google_user_keeps_disabled_status?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))

	legacy := &model.User{
		GoogleSub:  model.StringPtr("legacy-sub"),
		Email:      "legacy@example.com",
		Name:       "Legacy User",
		InviteCode: "invite-000001",
		Status:     model.UserStatusDisabled,
	}
	require.NoError(t, db.Create(legacy).Error)

	store := NewWithDB(db)
	user, err := store.SaveGoogleUser(context.Background(), "legacy-sub", "legacy@example.com", "Renamed User", nil)
	require.NoError(t, err)
	require.Equal(t, model.UserStatusDisabled, user.Status)

	saved, err := store.GetUserByID(context.Background(), legacy.ID)
	require.NoError(t, err)
	require.Equal(t, model.UserStatusDisabled, saved.Status)
	require.Equal(t, "Renamed User", saved.Name)
}

func TestListUsersFiltersByIDAndEmail(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:list_users_filters_by_id_and_email?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserHostedCreditAccount{}))

	paidAt := time.Date(2026, 6, 8, 7, 20, 0, 0, time.UTC)
	users := []model.User{
		{GoogleSub: model.StringPtr("sub-1"), Email: "alpha@example.com", Name: "Alpha", InviteCode: "invite-alpha", Status: model.UserStatusActive},
		{GoogleSub: model.StringPtr("sub-2"), Email: "beta@example.com", Name: "Beta", InviteCode: "invite-beta", Status: model.UserStatusActive, PaidEntitlement: true, PaidEntitlementUpdatedAt: &paidAt, PaidEntitlementSource: "stripe"},
		{GoogleSub: model.StringPtr("sub-3"), Email: "ops@example.org", Name: "Ops", InviteCode: "invite-ops", Status: model.UserStatusActive},
	}
	for i := range users {
		require.NoError(t, db.Create(&users[i]).Error)
	}
	require.NoError(t, db.Create(&model.UserHostedCreditAccount{UserID: users[1].ID, CreditBalance: 1250}).Error)

	store := NewWithDB(db)
	all, err := store.ListUsers(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, all, 3)
	require.Equal(t, 0, all[0].CreditBalance)
	require.Equal(t, 1250, all[1].CreditBalance)

	byID, err := store.ListUsers(context.Background(), strconv.FormatUint(users[1].ID, 10))
	require.NoError(t, err)
	require.Len(t, byID, 1)
	require.Equal(t, "beta@example.com", byID[0].Email)
	require.Equal(t, 1250, byID[0].CreditBalance)
	require.True(t, byID[0].PaidEntitlement)
	require.NotNil(t, byID[0].PaidEntitlementUpdatedAt)
	require.Equal(t, "stripe", byID[0].PaidEntitlementSource)

	byEmail, err := store.ListUsers(context.Background(), "example.com")
	require.NoError(t, err)
	require.Len(t, byEmail, 2)
	require.Equal(t, "beta@example.com", byEmail[0].Email)
	require.Equal(t, 1250, byEmail[0].CreditBalance)
	require.Equal(t, "alpha@example.com", byEmail[1].Email)
	require.Equal(t, 0, byEmail[1].CreditBalance)
}

func TestUserPaidEntitlementDefaultsFalseAndCanBeSet(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:user_paid_entitlement?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))

	user := &model.User{GoogleSub: model.StringPtr("paid-sub"), Email: "paid-store@example.com", Name: "Paid Store", InviteCode: "invite-paid-store", Status: model.UserStatusActive}
	require.NoError(t, db.Create(user).Error)

	store := NewWithDB(db)
	paid, err := store.GetUserPaidEntitlement(context.Background(), user.ID)
	require.NoError(t, err)
	require.False(t, paid)

	require.NoError(t, store.SetUserPaidEntitlement(context.Background(), user.ID, true, "admin"))
	paid, err = store.GetUserPaidEntitlement(context.Background(), user.ID)
	require.NoError(t, err)
	require.True(t, paid)

	saved, err := store.GetUserByID(context.Background(), user.ID)
	require.NoError(t, err)
	require.True(t, saved.PaidEntitlement)
	require.Equal(t, "admin", saved.PaidEntitlementSource)
	require.NotNil(t, saved.PaidEntitlementUpdatedAt)
}

func TestCountReferralsByInviterUserID(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:count_referrals_by_inviter_user_id?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.UserReferral{}))

	store := NewWithDB(db)
	require.NoError(t, db.Create(&model.UserReferral{InviterUserID: 7, InvitedUserID: 100, InviteCode: "invite-007"}).Error)
	require.NoError(t, db.Create(&model.UserReferral{InviterUserID: 7, InvitedUserID: 101, InviteCode: "invite-007"}).Error)
	require.NoError(t, db.Create(&model.UserReferral{InviterUserID: 8, InvitedUserID: 102, InviteCode: "invite-008"}).Error)

	count, err := store.CountReferralsByInviterUserID(context.Background(), 7)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
}

func TestRegisterReferralWithinLimitHonorsCapAndIdempotency(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:register_referral_within_limit?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserReferral{}))

	store := NewWithDB(db)
	inviter := &model.User{
		GoogleSub:  model.StringPtr("google-sub-1"),
		Email:      "demo@example.com",
		Name:       "Demo User",
		InviteCode: "invite-000001",
		Status:     model.UserStatusActive,
	}
	require.NoError(t, db.Create(inviter).Error)

	registeredAt := time.Now().UTC()
	for i := uint64(0); i < 5; i++ {
		referral, err := store.RegisterReferralWithinLimit(context.Background(), inviter.ID, 100+i, inviter.InviteCode, registeredAt)
		require.NoError(t, err)
		require.Equal(t, inviter.ID, referral.InviterUserID)
	}

	idempotent, err := store.RegisterReferralWithinLimit(context.Background(), inviter.ID, 100, inviter.InviteCode, registeredAt.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, uint64(100), idempotent.InvitedUserID)

	_, err = store.RegisterReferralWithinLimit(context.Background(), inviter.ID, 999, inviter.InviteCode, registeredAt.Add(2*time.Minute))
	require.ErrorIs(t, err, growthsvc.ErrInviteLimitReached)

	count, err := store.CountReferralsByInviterUserID(context.Background(), inviter.ID)
	require.NoError(t, err)
	require.EqualValues(t, 5, count)
}

func TestAppCreateAPIKeyUsesLeastPrivilegeDefaults(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:app_create_api_key_defaults?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.APIKey{}))

	store := NewWithDB(db)
	key, err := store.AppCreateAPIKey(context.Background(), 42, "Starter", "hash-1", "cop_test", "ciphertext-1")
	require.NoError(t, err)
	require.Equal(t, "external_only", key.AllowedModes)
	require.False(t, key.HostedEnabled)
	require.NotNil(t, key.DefaultRuntimeMode)
	require.Equal(t, "external", *key.DefaultRuntimeMode)
	require.NotNil(t, key.KeyCiphertext)
	require.Equal(t, "ciphertext-1", *key.KeyCiphertext)
}

func TestAdminCreateAPIKeyUsesLeastPrivilegeDefaults(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:admin_create_api_key_defaults?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.APIKey{}))

	store := NewWithDB(db)
	key, err := store.AdminCreateAPIKey(context.Background(), nil, "Ops", nil, nil, nil, "hash-2", "cop_admin", "ciphertext-2", nil)
	require.NoError(t, err)
	require.Equal(t, "external_only", key.AllowedModes)
	require.False(t, key.HostedEnabled)
	require.NotNil(t, key.DefaultRuntimeMode)
	require.Equal(t, "external", *key.DefaultRuntimeMode)
	require.NotNil(t, key.KeyCiphertext)
	require.Equal(t, "ciphertext-2", *key.KeyCiphertext)
}

func TestAddCreditBalanceToAPIKeyEnablesHostedEntitlement(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:add_credit_balance_enables_hosted?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.APIKey{}))

	store := NewWithDB(db)
	defaultRuntimeMode := "external"
	key := &model.APIKey{
		KeyHash:            "hash-3",
		KeyPrefix:          "cop_credit",
		Status:             model.APIKeyStatusActive,
		PlanName:           "Starter",
		AllowedModes:       "external_only",
		HostedEnabled:      false,
		DefaultRuntimeMode: &defaultRuntimeMode,
		CreditBalance:      0,
	}
	require.NoError(t, store.CreateAPIKey(context.Background(), key))

	updated, err := store.AddCreditBalanceToAPIKey(context.Background(), key.ID, 300)
	require.NoError(t, err)
	require.Equal(t, 300, updated.CreditBalance)
	require.True(t, updated.HostedEnabled)
	require.Equal(t, "hybrid", updated.AllowedModes)
	require.NotNil(t, updated.DefaultRuntimeMode)
	require.Equal(t, "hosted", *updated.DefaultRuntimeMode)
}

func TestGrantHostedCreditsToAPIKeyIsIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:grant_hosted_credits_idempotent?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.APIKey{}, &model.HostedCreditGrant{}))

	store := NewWithDB(db)
	user := &model.User{GoogleSub: model.StringPtr("sub-hosted"), Email: "hosted@example.com", Name: "Hosted", InviteCode: "invite-hosted", Status: model.UserStatusActive}
	require.NoError(t, db.Create(user).Error)
	defaultRuntimeMode := "external"
	key := &model.APIKey{
		OwnerUserID:        &user.ID,
		KeyHash:            "hash-hosted-grant",
		KeyPrefix:          "cop_hosted_grant",
		Status:             model.APIKeyStatusActive,
		PlanName:           "Starter",
		AllowedModes:       "external_only",
		HostedEnabled:      false,
		DefaultRuntimeMode: &defaultRuntimeMode,
	}
	require.NoError(t, store.CreateAPIKey(context.Background(), key))

	grant, created, err := store.GrantHostedCreditsToAPIKey(context.Background(), key.ID, user.ID, model.HostedCreditGrantSourceSignup, "signup-hosted-credits:1", 30, "new user signup hosted credits", "{}")
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, 30, grant.CreditAmount)

	grant, created, err = store.GrantHostedCreditsToAPIKey(context.Background(), key.ID, user.ID, model.HostedCreditGrantSourceSignup, "signup-hosted-credits:1", 30, "new user signup hosted credits", "{}")
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, 30, grant.CreditAmount)

	updated, err := store.FindAPIKeyByID(context.Background(), key.ID)
	require.NoError(t, err)
	require.Equal(t, 30, updated.CreditBalance)
	require.True(t, updated.HostedEnabled)
	require.Equal(t, "hybrid", updated.AllowedModes)
	require.NotNil(t, updated.DefaultRuntimeMode)
	require.Equal(t, "hosted", *updated.DefaultRuntimeMode)
}

func TestGrantHostedCreditsToUserAccountIsIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:grant_hosted_credits_to_user_account?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.UserHostedCreditAccount{}, &model.UserHostedCreditLedger{}))

	store := NewWithDB(db)
	grant, account, created, err := store.GrantHostedCreditsToUser(context.Background(), 42, model.HostedCreditLedgerSourceSignupBonus, "signup-hosted-credits:42", 100, "signup hosted credits", "{}")
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, 100, grant.CreditDelta)
	require.Equal(t, 100, account.CreditBalance)

	grant, account, created, err = store.GrantHostedCreditsToUser(context.Background(), 42, model.HostedCreditLedgerSourceSignupBonus, "signup-hosted-credits:42", 100, "signup hosted credits", "{}")
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, 100, grant.CreditDelta)
	require.Equal(t, 100, account.CreditBalance)

	var ledgers []model.UserHostedCreditLedger
	require.NoError(t, db.Find(&ledgers).Error)
	require.Len(t, ledgers, 1)
}

func TestFindAPIKeyByHashUsesOwnerAccountCredits(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:find_api_key_by_hash_uses_owner_account_credits?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.APIKey{}, &model.UserHostedCreditAccount{}, &model.UserHostedCreditLedger{}))

	store := NewWithDB(db)
	userID := uint64(51)
	require.NoError(t, db.Create(&model.User{ID: userID, GoogleSub: model.StringPtr("sub-license-key"), Email: "license-key@example.com", Name: "License Key", InviteCode: "invite-license-key", Status: model.UserStatusActive}).Error)
	defaultRuntimeMode := "hosted"
	key := &model.APIKey{
		OwnerUserID:        &userID,
		KeyHash:            "hash-license-key",
		KeyPrefix:          "cop_license",
		Status:             model.APIKeyStatusActive,
		PlanName:           "Starter",
		AllowedModes:       "hosted_only",
		HostedEnabled:      true,
		DefaultRuntimeMode: &defaultRuntimeMode,
		CreditBalance:      0,
	}
	require.NoError(t, store.CreateAPIKey(context.Background(), key))
	_, _, _, err = store.GrantHostedCreditsToUser(context.Background(), userID, model.HostedCreditLedgerSourcePurchase, "order:license-key", 120, "purchase", "{}")
	require.NoError(t, err)

	loaded, err := store.FindAPIKeyByHash(context.Background(), "hash-license-key")
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, 120, loaded.CreditBalance)
}

func TestFindAPIKeyByHashTreatsDisabledOwnerAsDisabledKey(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:find_api_key_by_hash_treats_disabled_owner_as_disabled_key?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.APIKey{}, &model.UserHostedCreditAccount{}))

	store := NewWithDB(db)
	user := &model.User{
		GoogleSub:  model.StringPtr("google-sub-1"),
		Email:      "demo@example.com",
		Name:       "Demo User",
		InviteCode: "invite-000001",
		Status:     model.UserStatusDisabled,
	}
	require.NoError(t, db.Create(user).Error)

	quotaTotal := 10
	key := &model.APIKey{
		OwnerUserID: &user.ID,
		KeyHash:     "hash-owner-disabled",
		KeyPrefix:   "cop_live_disabled",
		Status:      model.APIKeyStatusActive,
		PlanName:    "Starter",
		QuotaTotal:  &quotaTotal,
	}
	require.NoError(t, db.Create(key).Error)

	saved, err := store.FindAPIKeyByHash(context.Background(), key.KeyHash)
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.Equal(t, model.APIKeyStatusDisabled, saved.Status)
}

func TestConsumePaidQuotaByHashRejectsDisabledOwner(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:consume_paid_quota_by_hash_rejects_disabled_owner?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.APIKey{}))

	store := NewWithDB(db)
	user := &model.User{
		GoogleSub:  model.StringPtr("google-sub-1"),
		Email:      "demo@example.com",
		Name:       "Demo User",
		InviteCode: "invite-000001",
		Status:     model.UserStatusDisabled,
	}
	require.NoError(t, db.Create(user).Error)

	quotaTotal := 10
	key := &model.APIKey{
		OwnerUserID: &user.ID,
		KeyHash:     "hash-owner-disabled",
		KeyPrefix:   "cop_live_disabled",
		Status:      model.APIKeyStatusActive,
		PlanName:    "Starter",
		QuotaTotal:  &quotaTotal,
	}
	require.NoError(t, db.Create(key).Error)

	_, err = store.ConsumePaidQuotaByHash(context.Background(), key.KeyHash)
	require.Error(t, err)
	require.Contains(t, err.Error(), "api key is disabled")

	var saved model.APIKey
	require.NoError(t, db.First(&saved, key.ID).Error)
	require.Equal(t, 0, saved.QuotaUsed)
}

func TestUserAIGatewayAPIKeyLifecycleIsUniquePerUser(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:user_aigateway_api_key_lifecycle?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.UserAIGatewayAPIKey{}))

	store := NewWithDB(db)
	ctx := context.Background()

	creating, err := store.ClaimUserAIGatewayAPIKeyCreation(ctx, 42, "officecli-user-42")
	require.NoError(t, err)
	require.Equal(t, uint64(42), creating.UserID)
	require.Equal(t, model.UserAIGatewayAPIKeyStatusCreating, creating.Status)
	require.Equal(t, "officecli-user-42", creating.UpstreamName)
	require.True(t, creating.CreationClaimed)

	duplicate, err := store.ClaimUserAIGatewayAPIKeyCreation(ctx, 42, "officecli-user-42")
	require.NoError(t, err)
	require.Equal(t, creating.ID, duplicate.ID)
	require.False(t, duplicate.CreationClaimed)

	active, err := store.ActivateUserAIGatewayAPIKey(ctx, 42, "ciphertext-value", "sk-user", "upstream-123", "officecli-user-42")
	require.NoError(t, err)
	require.Equal(t, model.UserAIGatewayAPIKeyStatusActive, active.Status)
	require.Equal(t, "ciphertext-value", active.KeyCiphertext)
	require.Equal(t, "sk-user", active.KeyPrefix)
	require.Equal(t, "upstream-123", active.UpstreamID)
	require.Empty(t, active.LastError)

	found, err := store.FindUserAIGatewayAPIKeyByUserID(ctx, 42)
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, active.ID, found.ID)
	require.Equal(t, model.UserAIGatewayAPIKeyStatusActive, found.Status)
	require.Equal(t, "ciphertext-value", found.KeyCiphertext)
}

func TestUserAIGatewayAPIKeyCreationErrorCanBeRetried(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:user_aigateway_api_key_error_retry?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.UserAIGatewayAPIKey{}))

	store := NewWithDB(db)
	ctx := context.Background()

	_, err = store.ClaimUserAIGatewayAPIKeyCreation(ctx, 42, "officecli-user-42")
	require.NoError(t, err)

	failed, err := store.MarkUserAIGatewayAPIKeyCreationError(ctx, 42, "officecli-user-42", "gateway unavailable")
	require.NoError(t, err)
	require.Equal(t, model.UserAIGatewayAPIKeyStatusError, failed.Status)
	require.Equal(t, "gateway unavailable", failed.LastError)

	retry, err := store.ClaimUserAIGatewayAPIKeyCreation(ctx, 42, "officecli-user-42")
	require.NoError(t, err)
	require.Equal(t, failed.ID, retry.ID)
	require.Equal(t, model.UserAIGatewayAPIKeyStatusCreating, retry.Status)
	require.True(t, retry.CreationClaimed)
	require.Empty(t, retry.LastError)
}

func TestListUserHostedCreditLedgerFiltersZeroDeltaNoise(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:list_credit_ledger_filter?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.UserHostedCreditAccount{}, &model.UserHostedCreditLedger{}))

	store := NewWithDB(db)
	ctx := context.Background()
	userID := uint64(42)

	rows := []model.UserHostedCreditLedger{
		{UserID: userID, SourceType: model.HostedCreditLedgerSourceSignupBonus, IdempotencyKey: "signup:42", CreditDelta: 100, Reason: "signup bonus", MetadataJSON: "{}"},
		{UserID: userID, SourceType: model.HostedCreditLedgerSourceCharge, IdempotencyKey: "charge:c1", CreditDelta: -10, Reason: "charge", MetadataJSON: "{}"},
		{UserID: userID, SourceType: model.HostedCreditLedgerSourceChargeFailedPostUpstream, IdempotencyKey: "charge_failed:c2", CreditDelta: 0, Reason: "charge failed post upstream", MetadataJSON: "{}"},
		{UserID: userID, SourceType: model.HostedCreditLedgerSourcePurchase, IdempotencyKey: "order:1", CreditDelta: 500, Reason: "purchase", MetadataJSON: "{}"},
	}
	for i := range rows {
		require.NoError(t, db.Create(&rows[i]).Error)
	}

	filtered, err := store.ListUserHostedCreditLedger(ctx, userID, false)
	require.NoError(t, err)

	for _, entry := range filtered {
		if entry.CreditDelta == 0 {
			allowed := entry.SourceType == model.HostedCreditLedgerSourceCharge ||
				entry.SourceType == model.HostedCreditLedgerSourceChargeFailedPostUpstream
			require.True(t, allowed, "filtered list contains zero-delta row with unexpected source_type: %s (id=%d)", entry.SourceType, entry.ID)
		}
	}

	require.Equal(t, 4, len(filtered), "expected signup_bonus, charge, charge_failed, purchase")

	all, err := store.ListUserHostedCreditLedger(ctx, userID, true)
	require.NoError(t, err)
	require.Equal(t, 4, len(all), "include_zero_delta=true should return all rows")
}

func TestImagePromptTemplatesListFiltersEnabledAndOrders(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:image_prompt_templates?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ImagePromptTemplate{}))
	store := NewWithDB(db)

	require.NoError(t, store.CreateImagePromptTemplate(context.Background(), &model.ImagePromptTemplate{Slug: "disabled", Title: "Disabled", PromptPreset: "disabled prompt", SortOrder: 1, Enabled: false}))
	require.NoError(t, store.CreateImagePromptTemplate(context.Background(), &model.ImagePromptTemplate{Slug: "second", Title: "Second", PromptPreset: "second prompt", SortOrder: 20, Enabled: true}))
	require.NoError(t, store.CreateImagePromptTemplate(context.Background(), &model.ImagePromptTemplate{Slug: "first", Title: "First", PromptPreset: "first prompt", SortOrder: 10, Enabled: true}))

	items, err := store.ListImagePromptTemplates(context.Background(), true)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, "first", items[0].Slug)
	require.Equal(t, "second", items[1].Slug)

	all, err := store.ListImagePromptTemplates(context.Background(), false)
	require.NoError(t, err)
	require.Len(t, all, 3)
}
