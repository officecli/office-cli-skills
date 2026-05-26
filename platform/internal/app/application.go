package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	ego "github.com/gotomicro/ego"
	"github.com/gotomicro/ego/server/egin"
	sdkoffice "github.com/officesdk/go-sdk/officesdk"
	"golang.org/x/time/rate"

	"github.com/officecli/officecli/platform/internal/admin"
	"github.com/officecli/officecli/platform/internal/apikey"
	"github.com/officecli/officecli/platform/internal/appuser"
	"github.com/officecli/officecli/platform/internal/auth"
	"github.com/officecli/officecli/platform/internal/billing"
	"github.com/officecli/officecli/platform/internal/clisession"
	"github.com/officecli/officecli/platform/internal/discordoauth"
	growthsvc "github.com/officecli/officecli/platform/internal/growth"
	"github.com/officecli/officecli/platform/internal/hostedllm"
	"github.com/officecli/officecli/platform/internal/httpapi"
	"github.com/officecli/officecli/platform/internal/issuereports"
	licensesvc "github.com/officecli/officecli/platform/internal/license"
	"github.com/officecli/officecli/platform/internal/model"
	"github.com/officecli/officecli/platform/internal/objectstore"
	"github.com/officecli/officecli/platform/internal/officesdk"
	"github.com/officecli/officecli/platform/internal/operations"
	"github.com/officecli/officecli/platform/internal/previewshare"
	publishsvc "github.com/officecli/officecli/platform/internal/publish"
	rewardsvc "github.com/officecli/officecli/platform/internal/reward"
	redemptionsvc "github.com/officecli/officecli/platform/internal/redemption"
	redisstore "github.com/officecli/officecli/platform/internal/store/redis"
	sqlstore "github.com/officecli/officecli/platform/internal/store/sqlstore"
)

type Application struct{ ego *ego.Ego }

type redisLicenseAdapter struct{ store *redisstore.Store }

type appAuthResolver func(cookieValue string) (uint64, string, error)

type discordOAuthRouteService interface {
	LoginURL(ctx context.Context, userID uint64, returnTo string) (string, error)
	HandleCallback(ctx context.Context, code, state string) (*discordoauth.CallbackResult, error)
}

type authRouteService interface {
	LoginURL(ctx context.Context, returnTo, inviteCode string) (string, error)
	HandleCallback(ctx context.Context, code, state string) (*model.User, string, string, error)
	GitHubEnabled() bool
	GitHubLoginURL(ctx context.Context, returnTo, inviteCode string) (string, error)
	HandleGitHubCallback(ctx context.Context, code, state string) (*model.User, string, string, error)
	Me(ctx context.Context, raw string) (*model.User, error)
	Logout(ctx context.Context, raw string) error
}

type adminRouteService interface {
	ResolveSession(raw string) (string, error)
	LoginURL(ctx context.Context, returnTo string) (string, error)
	HandleGoogleCallback(ctx context.Context, code, state string) (*admin.AdminIdentity, string, string, error)
	MockGoogleLogin(ctx context.Context, email, name string) (*admin.AdminIdentity, string, error)
	Login(ctx context.Context, password string) (string, error)
	CurrentIdentity(ctx context.Context, rawCookie string) (*admin.AdminIdentity, error)
	Logout(ctx context.Context, rawCookie string) error
	Overview(ctx context.Context) (*model.OverviewStats, error)
	FingerprintQuality(ctx context.Context) (*model.FingerprintQuality, error)
	OperationsFunnel(ctx context.Context, windowStart, now time.Time) (*model.OperationsFunnel, error)
	ListAPIKeys(ctx context.Context, ownerUserID *uint64) ([]model.APIKey, error)
	GetAPIKeyPlaintext(ctx context.Context, id uint64, actor string) (string, error)
	CreateAPIKey(ctx context.Context, req admin.CreateAPIKeyRequest) (*admin.CreateAPIKeyResponse, *model.APIKey, error)
	UpdateAPIKey(ctx context.Context, id uint64, req admin.UpdateAPIKeyRequest) error
	ListUsageEvents(ctx context.Context, filter sqlstore.UsageEventFilter) ([]model.UsageEvent, error)
	GetPreference(ctx context.Context, adminEmail, pageKey string) (*model.AdminUserPreference, error)
	SavePreference(ctx context.Context, adminEmail, pageKey, preferencesJSON string) (*model.AdminUserPreference, error)
	ListUsers(ctx context.Context, query string) ([]model.User, error)
	UpdateUser(ctx context.Context, id uint64, req admin.UpdateUserRequest) error
	ListOrders(ctx context.Context) ([]model.Order, error)
	UpdateOrder(ctx context.Context, id uint64, req admin.UpdateOrderRequest) error
	ListBillingEvents(ctx context.Context) ([]model.BillingEvent, error)
	ListCreditLedger(ctx context.Context, includeZeroDelta bool) ([]model.UserHostedCreditLedger, error)
	Growth(ctx context.Context) (*admin.GrowthSnapshot, error)
	QuotaSources(ctx context.Context, filter admin.QuotaSourcesFilter) (*admin.QuotaSources, error)
	HostedPricingRules(ctx context.Context) ([]model.HostedPricingRule, error)
	HostedBillingConfig(ctx context.Context) (*admin.HostedBillingConfig, error)
	UpdateHostedPricingSettings(ctx context.Context, req admin.UpdateHostedPricingSettingsRequest) (*model.HostedPricingSetting, error)
	CreateHostedModelPricingConfig(ctx context.Context, req admin.UpsertHostedModelPricingConfigRequest) (*model.HostedModelPricingConfig, error)
	UpdateHostedModelPricingConfig(ctx context.Context, id uint64, req admin.UpsertHostedModelPricingConfigRequest) (*model.HostedModelPricingConfig, error)
	CreateHostedPricingRule(ctx context.Context, req admin.UpsertHostedPricingRuleRequest) (*model.HostedPricingRule, error)
	UpdateHostedPricingRule(ctx context.Context, id uint64, req admin.UpsertHostedPricingRuleRequest) (*model.HostedPricingRule, error)
	CreateHostedCreditPack(ctx context.Context, req admin.UpsertHostedCreditPackRequest) (*model.HostedCreditPack, error)
	UpdateHostedCreditPack(ctx context.Context, id uint64, req admin.UpsertHostedCreditPackRequest) (*model.HostedCreditPack, error)
	CreateRedemptionCode(ctx context.Context, actorEmail string, req admin.CreateRedemptionCodeRequest) (*model.RedemptionCode, error)
	ListRedemptionCodes(ctx context.Context, req admin.ListRedemptionCodesRequest) (*admin.ListRedemptionCodesResponse, error)
	GetRedemptionCode(ctx context.Context, id uint64) (*model.RedemptionCode, error)
	UpdateRedemptionCode(ctx context.Context, actorEmail string, id uint64, req admin.UpdateRedemptionCodeRequest) (*model.RedemptionCode, error)
	EnableRedemptionCode(ctx context.Context, actorEmail string, id uint64) (*model.RedemptionCode, error)
	DisableRedemptionCode(ctx context.Context, actorEmail string, id uint64) (*model.RedemptionCode, error)
	ListRedemptionRecords(ctx context.Context, req admin.ListRedemptionRecordsRequest) (*admin.ListRedemptionRecordsResponse, error)
}

type stripeRouteService interface {
	HandleWebhook(ctx context.Context, payload []byte, signature string) error
}

func (r redisLicenseAdapter) GetConsumeResult(ctx context.Context, requestID string) (*licensesvc.ConsumeResponse, error) {
	var resp licensesvc.ConsumeResponse
	ok, err := r.store.GetJSON(ctx, "consume:"+requestID, &resp)
	if err != nil || !ok {
		return nil, err
	}
	return &resp, nil
}
func (r redisLicenseAdapter) SaveConsumeResult(ctx context.Context, requestID string, resp *licensesvc.ConsumeResponse, ttl time.Duration) error {
	return r.store.SetJSON(ctx, "consume:"+requestID, resp, ttl)
}
func (r redisLicenseAdapter) AcquireConsumeLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return r.store.AcquireLock(ctx, key, ttl)
}
func (r redisLicenseAdapter) ReleaseConsumeLock(ctx context.Context, key string) error {
	return r.store.ReleaseLock(ctx, key)
}

func New() (*Application, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	dbStore, err := sqlstore.New(cfg.PostgresDSN)
	if err != nil {
		return nil, err
	}
	if err := dbStore.Ping(context.Background()); err != nil {
		return nil, err
	}
	if err := dbStore.EnsureMigrations(context.Background()); err != nil {
		return nil, err
	}
	redisClient := redisstore.NewClient(cfg.RedisAddr)
	redisRepo := redisstore.NewStore(redisClient)
	if err := redisRepo.Ping(context.Background()); err != nil {
		return nil, err
	}
	previewObjects, err := objectstore.New(objectstore.Config{
		Endpoint:  cfg.PreviewObjectEndpoint,
		AccessKey: cfg.PreviewObjectAccessKey,
		SecretKey: cfg.PreviewObjectSecretKey,
		Bucket:    cfg.PreviewObjectBucket,
		UseSSL:    cfg.PreviewObjectUseSSL,
	})
	if err != nil {
		return nil, err
	}

	rewardService := rewardsvc.NewService(dbStore)
	growthService := growthsvc.NewService(dbStore, dbStore, dbStore, dbStore, dbStore)
	apiKeyCipher, err := apikey.NewCipher(cfg.APIKeyEncryptionKey)
	if err != nil {
		return nil, err
	}
	lic := licensesvc.NewService(
		apiKeyRepo{store: dbStore},
		dbStore,
		nil,
		usageEventRepo{store: dbStore},
		redisLicenseAdapter{store: redisRepo},
		rewardService,
		growthService,
		cfg.APIKeyHashSalt,
		cfg.UsageIdempotencyTTL,
		licensesvc.ProofConfig{
			Seed: cfg.LicenseProofSeed,
			TTL:  cfg.LicenseProofTTL,
		},
	)
	hostedLLMSvc := hostedllm.NewService(dbStore, hostedllm.Config{
		BaseURL:                cfg.HostedLLMBaseURL,
		APIKey:                 cfg.HostedLLMAPIKey,
		TextModel:              cfg.HostedLLMTextModel,
		ImageModel:             cfg.HostedLLMImageModel,
		Provider:               cfg.HostedLLMProvider,
		HashSalt:               cfg.APIKeyHashSalt,
		ModelConfigs:           cfg.HostedModelPricingConfigs,
		Rules:                  cfg.HostedPricingRules,
		TimeoutSec:             cfg.HostedLLMTimeoutSec,
		AIGatewayAdminBaseURL:  cfg.AIGatewayAdminBaseURL,
		AIGatewayAdminAPIKey:   cfg.AIGatewayAdminAPIKey,
		AIGatewayAPIKeyGroup:   cfg.AIGatewayAPIKeyGroup,
		AIGatewayCreateKeyPath: cfg.AIGatewayCreateAPIKeyPath,
		AIGatewayKeyCipher:     apiKeyCipher,
		ReconcileEnabled:       cfg.HostedReconcileEnabled,
	}, lic)
	adminProvider := newAdminOAuthProvider(cfg)
	adminSvc := admin.NewService(dbStore, redisRepo, cfg.AdminPassword, cfg.AdminSessionTTL, "cop_admin_session", admin.NewSecureCookieCodec(cfg.SessionSecret), cfg.APIKeyHashSalt, apiKeyCipher, adminProvider, cfg.AdminGoogleAllowlist, hostedLLMSvc)
	adminSvc.UseMockData(cfg.AdminMockDataEnabled && cfg.AppEnv == "development")
	redemptionSvc := redemptionsvc.NewService(dbStore)
	adminSvc.SetRedemptionService(redemptionSvc)
	authSvc := auth.NewService(newAppOAuthProvider(cfg), dbStore, redisRepo, "cop_app_session", cfg.AppSessionTTL, auth.NewSecureCookieCodec(cfg.AppSessionSecret), growthService, cfg.AppGoogleAllowlist)
	if strings.TrimSpace(cfg.GitHubClientID) != "" && strings.TrimSpace(cfg.GitHubClientSecret) != "" {
		authSvc.WithGitHubProvider(
			auth.NewGitHubOAuthProvider(cfg.GitHubClientID, cfg.GitHubClientSecret, cfg.GitHubRedirectURL),
			cfg.AppGitHubAllowlist,
		)
	}
	billingSvc := billing.NewService(dbStore, billing.NewStripeGateway(cfg.StripeSecretKey, cfg.StripeWebhookSecret, cfg.StripeSuccessURL, cfg.StripeCancelURL), cfg.PricingPacks)
	appSvc := appuser.NewService(dbStore, billingSvc, cfg.APIKeyHashSalt, apiKeyCipher, growthService)
	appSvc.UseMockData(cfg.AppMockDataEnabled && cfg.AppEnv == "development")
	lic.SetAnonymousGranter(appSvc)
	fileStore := officesdk.NewFileStore(redisRepo)
	previewShares := previewshare.NewService(dbStore.DB(), cfg.AppSessionSecret, cfg.AppSessionCookieDomain, fileStore, previewObjects)
	sdkProvider := officesdk.NewFileProvider(fileStore, previewObjects, previewShares)
	sdkHandler := officesdk.NewHandler(fileStore, sdkProvider, cfg.OfficeSDKEndpoint, cfg.OfficeSDKJWTSecret)
	publishService := publishsvc.NewService(apiKeyRepo{store: dbStore}, previewObjects, fileStore, previewShares, publishsvc.Config{
		SiteBaseURL:          cfg.SiteBaseURL,
		HashSalt:             cfg.APIKeyHashSalt,
		DefaultExpireSeconds: cfg.PublishDefaultExpireSeconds,
	})
	discordOAuthSvc := discordoauth.NewService(
		discordoauth.NewOAuthClient(cfg.DiscordClientID, cfg.DiscordClientSecret, cfg.DiscordRedirectURL, cfg.DiscordGuildID, cfg.DiscordBotToken),
		redisRepo,
		growthService,
	)
	cliSessionSvc := clisession.NewService(dbStore, cfg.PlatformBaseURL)
	operationsSvc := operations.NewService(dbStore)
	issueReportsSvc := issuereports.NewService(dbStore, issuereports.NewMetrics(slog.Default()), time.Now)

	port, err := parsePort(cfg.HTTPAddr)
	if err != nil {
		return nil, err
	}
	server := egin.DefaultContainer().Build(egin.WithHost(hostPart(cfg.HTTPAddr)), egin.WithPort(port))
	registerRoutesWithHosted(server, cfg, lic, adminSvc, authSvc, appSvc, billingSvc, hostedLLMSvc, publishService, discordOAuthSvc, cliSessionSvc, previewShares, sdkHandler, sdkProvider, operationsSvc, redemptionSvc, issueReportsSvc)

	application := ego.New()
	application.Serve(server)
	return &Application{ego: application}, nil
}

func newAppOAuthProvider(cfg Config) auth.OAuthProvider {
	return newOAuthProvider(cfg, cfg.OAuth2ClientID, cfg.OAuth2ClientSecret, cfg.OAuth2RedirectURL, cfg.GoogleRedirectURL)
}

func newAdminOAuthProvider(cfg Config) auth.OAuthProvider {
	return newOAuthProvider(cfg, cfg.AdminOAuth2ClientID, cfg.AdminOAuth2ClientSecret, cfg.AdminOAuth2RedirectURL, cfg.AdminGoogleRedirectURL)
}

func newOAuthProvider(cfg Config, oauth2ClientID, oauth2ClientSecret, oauth2RedirectURL, googleRedirectURL string) auth.OAuthProvider {
	if strings.TrimSpace(cfg.OAuth2AuthURL) != "" || strings.TrimSpace(cfg.OAuth2TokenURL) != "" || strings.TrimSpace(cfg.OAuth2UserinfoURL) != "" {
		return auth.NewOAuth2Provider(auth.OAuth2ProviderConfig{
			ClientID:     oauth2ClientID,
			ClientSecret: oauth2ClientSecret,
			RedirectURL:  oauth2RedirectURL,
			AuthURL:      cfg.OAuth2AuthURL,
			TokenURL:     cfg.OAuth2TokenURL,
			UserinfoURL:  cfg.OAuth2UserinfoURL,
			Scopes:       cfg.OAuth2Scopes,
			SubjectField: cfg.OAuth2SubjectField,
			EmailField:   cfg.OAuth2EmailField,
			NameField:    cfg.OAuth2NameField,
			AvatarField:  cfg.OAuth2AvatarField,
		})
	}
	return auth.NewGoogleOAuthProvider(cfg.GoogleClientID, cfg.GoogleClientSecret, googleRedirectURL)
}

func (a *Application) Run() error { return a.ego.Run() }

func registerRoutes(r *egin.Component, cfg Config, lic *licensesvc.Service, adminSvc *admin.Service, authSvc *auth.Service, appSvc *appuser.Service, billingSvc *billing.Service, discordSvc discordOAuthRouteService) {
	registerRoutesWithHosted(r, cfg, lic, adminSvc, authSvc, appSvc, billingSvc, nil, nil, discordSvc, nil, nil, nil, nil, nil, nil, nil)
}

func registerRoutesWithHosted(r *egin.Component, cfg Config, lic *licensesvc.Service, adminSvc *admin.Service, authSvc *auth.Service, appSvc *appuser.Service, billingSvc *billing.Service, hostedSvc *hostedllm.Service, publishService publishRouteService, discordSvc discordOAuthRouteService, cliSessionSvc *clisession.Service, previewShares *previewshare.Service, sdkHandler *officesdk.Handler, sdkProvider *officesdk.FileProvider, operationsSvc *operations.Service, redemptionSvc *redemptionsvc.Service, issueReportsSvc *issuereports.Service) {
	r.Use(httpapi.RequestIDMiddleware())
	r.Use(httpapi.AccessLogMiddleware(time.Second))
	r.Use(gin.Recovery())
	r.GET("/healthz", func(c *gin.Context) { httpapi.JSON(c, http.StatusOK, gin.H{"status": "ok"}) })
	api := r.Group("/api")
	api.Use(siteCORSMiddleware(cfg))

	registerLicenseRoutesWithConfig(api, cfg, lic, cliSessionSvc)
	registerHostedLLMRoutes(api, hostedSvc)
	registerPublishRoutes(api, cfg, publishService)
	registerOperationsRoutes(api, cfg, authSvc, operationsSvc)
	registerAuthRoutes(api, cfg, authSvc)
	registerCLIRoutes(api, cfg, authSvc, appSvc, cliSessionSvc)
	registerAdminRoutes(api, cfg, adminSvc)
	registerRedemptionUserRoutes(api, redemptionSvc, authSvc, cliSessionSvc)
	api.GET("/pricing", func(c *gin.Context) { httpapi.JSON(c, http.StatusOK, billingSvc.Pricing()) })
	registerAppRoutes(api, cfg, authSvc, appSvc, billingSvc, discordSvc, cliSessionSvc)
	registerStripeRoutes(api, billingSvc)
	registerIssueReportRoutes(api, cfg, cliSessionSvc, issueReportsSvc)
	registerPreviewRoutes(r, cfg, authSvc, previewShares, sdkHandler, sdkProvider)
	registerStatic(r.Engine, cfg)
}

type publishRouteService interface {
	Publish(ctx context.Context, bearer string, req publishsvc.Request) (*publishsvc.Result, error)
}

func registerPublishRoutes(api *gin.RouterGroup, cfg Config, publisher publishRouteService) {
	if publisher == nil {
		return
	}
	limit := cfg.PublishRateLimitPerMinute
	if limit <= 0 {
		limit = 30
	}
	ttl := cfg.RateLimitVisitorTTL
	if ttl <= 0 {
		ttl = defaultRateLimitVisitorTTL(cfg.AppEnv)
	}
	maxFileBytes := cfg.PublishMaxFileBytes
	if maxFileBytes <= 0 {
		maxFileBytes = 50 << 20
	}
	limiter := httpapi.NewRateLimitMiddleware(rate.Every(time.Minute/time.Duration(limit)), limit, ttl)
	api.POST("/publish", limiter, func(c *gin.Context) {
		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		if authHeader == "" {
			httpapi.AbortUnauthorized(c)
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxFileBytes+1<<20)
		reader, err := c.Request.MultipartReader()
		if err != nil {
			httpapi.Error(c, http.StatusBadRequest, "invalid multipart request")
			return
		}

		var (
			documentType     string
			documentName     string
			expiresInSeconds int
			fileName         string
			contentType      string
			fileData         []byte
		)

		for {
			part, err := reader.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				httpapi.Error(c, http.StatusBadRequest, "invalid multipart request")
				return
			}
			switch part.FormName() {
			case "file":
				fileName = filepath.Base(part.FileName())
				contentType = strings.TrimSpace(part.Header.Get("Content-Type"))
				fileData, err = io.ReadAll(part)
				_ = part.Close()
				if err != nil {
					httpapi.Error(c, http.StatusBadRequest, "invalid multipart request")
					return
				}
			case "document_type":
				data, _ := io.ReadAll(io.LimitReader(part, 128))
				documentType = strings.TrimSpace(string(data))
				_ = part.Close()
			case "document_name":
				data, _ := io.ReadAll(io.LimitReader(part, 512))
				documentName = strings.TrimSpace(string(data))
				_ = part.Close()
			case "expires_in_seconds":
				data, _ := io.ReadAll(io.LimitReader(part, 64))
				expiresInSeconds, _ = strconv.Atoi(strings.TrimSpace(string(data)))
				_ = part.Close()
			default:
				_, _ = io.Copy(io.Discard, part)
				_ = part.Close()
			}
		}
		if len(fileData) == 0 && fileName == "" {
			httpapi.Error(c, http.StatusBadRequest, "file is required")
			return
		}
		if documentName == "" {
			documentName = fileName
		}

		result, err := publisher.Publish(c.Request.Context(), authHeader, publishsvc.Request{
			FileName:         fileName,
			DocumentType:     documentType,
			DocumentName:     documentName,
			ExpiresInSeconds: expiresInSeconds,
			ContentType:      contentType,
			Reader:           bytes.NewReader(fileData),
		})
		if err != nil {
			status := http.StatusBadRequest
			errMsg := strings.ToLower(err.Error())
			switch {
			case strings.Contains(errMsg, "missing api key"), strings.Contains(errMsg, "invalid api key"), strings.Contains(errMsg, "disabled"), strings.Contains(errMsg, "cli session"):
				status = http.StatusUnauthorized
			case strings.Contains(errMsg, "too large"):
				status = http.StatusRequestEntityTooLarge
			}
			httpapi.Error(c, status, err.Error())
			return
		}
		c.JSON(http.StatusOK, result)
	})
}

func registerLicenseRoutesWithConfig(api *gin.RouterGroup, cfg Config, lic *licensesvc.Service, cliSvc *clisession.Service) {
	licenseRatePerMinute := cfg.LicenseRateLimitPerMinute
	if licenseRatePerMinute <= 0 {
		licenseRatePerMinute = defaultLicenseRateLimit(cfg.AppEnv)
	}
	rateLimitTTL := cfg.RateLimitVisitorTTL
	if rateLimitTTL <= 0 {
		rateLimitTTL = defaultRateLimitVisitorTTL(cfg.AppEnv)
	}
	licenseLimiter := httpapi.NewRateLimitMiddleware(
		rate.Every(time.Minute/time.Duration(licenseRatePerMinute)),
		licenseRatePerMinute,
		rateLimitTTL,
	)
	api.POST("/license/check", licenseLimiter, func(c *gin.Context) {
		var req licensesvc.CheckRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		req.AuditContext = usageAuditContext(c)
		if req.APIKey == "" && cliSvc != nil {
			if session, err := cliSvc.Resolve(c.Request.Context(), bearerToken(c.GetHeader("Authorization"))); err == nil && session != nil {
				req.UserID = session.UserID
			}
		}
		resp, err := lic.Check(c.Request.Context(), req)
		if err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, resp)
	})
	api.POST("/license/consume", licenseLimiter, func(c *gin.Context) {
		var req licensesvc.ConsumeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		req.AuditContext = usageAuditContext(c)
		if req.APIKey == "" && cliSvc != nil {
			if session, err := cliSvc.Resolve(c.Request.Context(), bearerToken(c.GetHeader("Authorization"))); err == nil && session != nil {
				req.UserID = session.UserID
			}
		}
		resp, err := lic.Consume(c.Request.Context(), req)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, licensesvc.ErrQuotaExhausted) || strings.Contains(err.Error(), "quota exhausted") {
				status = http.StatusConflict
			}
			httpapi.Error(c, status, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, resp)
	})
}

func registerLicenseRoutes(api *gin.RouterGroup, lic *licensesvc.Service) {
	registerLicenseRoutesWithConfig(api, Config{
		AppEnv:                    "production",
		LicenseRateLimitPerMinute: 30,
		RateLimitVisitorTTL:       5 * time.Minute,
	}, lic, nil)
}

func usageAuditContext(c *gin.Context) model.UsageAuditContext {
	if c == nil || c.Request == nil {
		return model.UsageAuditContext{}
	}
	return model.UsageAuditContext{
		ClientIP:      c.ClientIP(),
		ForwardedFor:  c.GetHeader("X-Forwarded-For"),
		UserAgent:     c.GetHeader("User-Agent"),
		RequestHost:   c.Request.Host,
		RequestPath:   c.Request.URL.Path,
		RequestMethod: c.Request.Method,
	}
}

func registerHostedLLMRoutes(api *gin.RouterGroup, hostedSvc *hostedllm.Service) {
	if hostedSvc == nil {
		return
	}
	llmAPI := api.Group("/llm/v1")
	llmAPI.POST("/text", func(c *gin.Context) {
		var req hostedllm.CompletionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		req.RequestID = c.GetHeader("X-Request-Id")
		req.Kind = "text"
		req.AuditContext = usageAuditContext(c)
		resp, err := hostedSvc.Complete(c.Request.Context(), c.GetHeader("Authorization"), c.GetHeader("X-Fingerprint-Hash"), req)
		if err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"content": resp.Content, "credit_balance": resp.CreditBalance, "credits_charged": resp.CreditsCharged})
	})
	llmAPI.POST("/json", func(c *gin.Context) {
		var req hostedllm.CompletionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		req.RequestID = c.GetHeader("X-Request-Id")
		req.Kind = "json"
		req.JSONMode = true
		req.AuditContext = usageAuditContext(c)
		resp, err := hostedSvc.Complete(c.Request.Context(), c.GetHeader("Authorization"), c.GetHeader("X-Fingerprint-Hash"), req)
		if err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"content": resp.Content, "credit_balance": resp.CreditBalance, "credits_charged": resp.CreditsCharged})
	})
	llmAPI.POST("/structured", func(c *gin.Context) {
		var req hostedllm.CompletionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		req.RequestID = c.GetHeader("X-Request-Id")
		req.Kind = "structured"
		req.AuditContext = usageAuditContext(c)
		resp, err := hostedSvc.Complete(c.Request.Context(), c.GetHeader("Authorization"), c.GetHeader("X-Fingerprint-Hash"), req)
		if err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"content": resp.Content, "credit_balance": resp.CreditBalance, "credits_charged": resp.CreditsCharged})
	})
	llmAPI.POST("/image", func(c *gin.Context) {
		var req hostedllm.ImageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		req.RequestID = c.GetHeader("X-Request-Id")
		req.AuditContext = usageAuditContext(c)
		resp, err := hostedSvc.GenerateImage(c.Request.Context(), c.GetHeader("Authorization"), c.GetHeader("X-Fingerprint-Hash"), req)
		if err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"data":                 base64.StdEncoding.EncodeToString(resp.Data),
			"mime":                 resp.MIME,
			"credit_balance":       resp.CreditBalance,
			"credits_charged":      resp.CreditsCharged,
			"access_mode":          resp.AccessMode,
			"remaining":            resp.Remaining,
			"reward_remaining":     resp.RewardRemaining,
			"paid_quota_remaining": resp.PaidQuotaRemaining,
		})
	})
}

func registerOperationsRoutes(api *gin.RouterGroup, cfg Config, authSvc authRouteService, operationsSvc *operations.Service) {
	if operationsSvc == nil {
		return
	}
	limitPerMinute := 120
	rateLimitTTL := cfg.RateLimitVisitorTTL
	if rateLimitTTL <= 0 {
		rateLimitTTL = defaultRateLimitVisitorTTL(cfg.AppEnv)
	}
	limiter := httpapi.NewRateLimitMiddleware(rate.Every(time.Minute/time.Duration(limitPerMinute)), limitPerMinute, rateLimitTTL)
	api.POST("/events/track", limiter, func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				httpapi.Error(c, http.StatusRequestEntityTooLarge, "request body too large")
				return
			}
			httpapi.Error(c, http.StatusBadRequest, "invalid JSON body")
			return
		}
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		var req operations.TrackRequest
		if err := decoder.Decode(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			httpapi.Error(c, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if strings.TrimSpace(req.VisitorID) == "" {
			req.VisitorID = c.Query("visitor_id")
		}
		if strings.TrimSpace(req.Invite) == "" {
			req.Invite = c.Query("invite")
		}
		fillOperationalUTMFromQuery(&req, c)

		var userID *uint64
		if authSvc != nil {
			if raw, err := c.Cookie("cop_app_session"); err == nil && raw != "" {
				if resolver, ok := authSvc.(interface {
					ResolveSession(string) (*auth.SessionPayload, error)
				}); ok {
					if payload, err := resolver.ResolveSession(raw); err == nil && payload != nil && payload.UserID > 0 {
						id := payload.UserID
						userID = &id
					}
				}
			}
		}

		result, err := operationsSvc.Track(c.Request.Context(), req, operations.TrackContext{
			UserID:    userID,
			Host:      c.Request.Host,
			Secure:    shouldUseVisitorSecureCookie(c, cfg),
			UserAgent: c.GetHeader("User-Agent"),
		})
		if err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		if result != nil && result.VisitorCookie != nil {
			http.SetCookie(c.Writer, result.VisitorCookie)
		}
		httpapi.JSON(c, http.StatusOK, gin.H{"success": true})
	})
}

func fillOperationalUTMFromQuery(req *operations.TrackRequest, c *gin.Context) {
	if strings.TrimSpace(req.UTMSource) == "" {
		req.UTMSource = c.Query("utm_source")
	}
	if strings.TrimSpace(req.UTMMedium) == "" {
		req.UTMMedium = c.Query("utm_medium")
	}
	if strings.TrimSpace(req.UTMCampaign) == "" {
		req.UTMCampaign = c.Query("utm_campaign")
	}
	if strings.TrimSpace(req.UTMTerm) == "" {
		req.UTMTerm = c.Query("utm_term")
	}
	if strings.TrimSpace(req.UTMContent) == "" {
		req.UTMContent = c.Query("utm_content")
	}
	if strings.TrimSpace(req.UTMID) == "" {
		req.UTMID = c.Query("utm_id")
	}
}

func shouldUseVisitorSecureCookie(c *gin.Context, cfg Config) bool {
	if c != nil && c.Request != nil && c.Request.TLS != nil {
		return true
	}
	if c != nil && strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		return true
	}
	return cfg.AppEnv == "production"
}

func registerAuthRoutes(api *gin.RouterGroup, cfg Config, authSvc authRouteService) {
	login := func(c *gin.Context) {
		returnTo := c.Query("return_to")
		inviteCode := c.Query("invite")
		if cfg.AppGoogleMockEnabled && cfg.AppEnv == "development" {
			mockSvc, ok := authSvc.(interface {
				MockGoogleLogin(ctx context.Context, email, name, returnTo string) (*model.User, string, string, error)
			})
			if !ok {
				httpapi.Error(c, http.StatusInternalServerError, "app mock google login is unavailable")
				return
			}
			_, rawCookie, redirectTo, err := mockSvc.MockGoogleLogin(c.Request.Context(), cfg.AppGoogleMockEmail, cfg.AppGoogleMockName, returnTo)
			if err != nil {
				httpapi.Error(c, http.StatusInternalServerError, err.Error())
				return
			}
			setSessionCookie(c, cfg, "cop_app_session", rawCookie, cfg.AppSessionTTL)
			c.Redirect(http.StatusFound, redirectTo)
			return
		}
		url, err := authSvc.LoginURL(c.Request.Context(), returnTo, inviteCode)
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		c.Redirect(http.StatusFound, url)
	}
	callback := func(c *gin.Context) {
		user, rawCookie, returnTo, err := authSvc.HandleCallback(c.Request.Context(), c.Query("code"), c.Query("state"))
		if err != nil {
			var denied *auth.AccessDeniedError
			if errors.As(err, &denied) {
				deniedURL := "/app/access-denied"
				if denied.Email != "" {
					deniedURL += "?email=" + url.QueryEscape(denied.Email)
				}
				c.Redirect(http.StatusFound, deniedURL)
				return
			}
			httpapi.LogWarnRequest(c, "auth_callback_failed",
				"path", c.Request.URL.Path,
				"client_ip", c.ClientIP(),
				"error", err.Error(),
			)
			httpapi.Error(c, http.StatusUnauthorized, err.Error())
			return
		}
		setSessionCookie(c, cfg, "cop_app_session", rawCookie, cfg.AppSessionTTL)
		_ = user
		c.Redirect(http.StatusFound, returnTo)
	}
	api.GET("/auth/oauth2/login", login)
	api.GET("/auth/oauth2/callback", callback)
	api.GET("/auth/google/login", login)
	api.GET("/auth/google/callback", callback)
	if authSvc.GitHubEnabled() {
		githubLogin := func(c *gin.Context) {
			returnTo := c.Query("return_to")
			inviteCode := c.Query("invite")
			u, err := authSvc.GitHubLoginURL(c.Request.Context(), returnTo, inviteCode)
			if err != nil {
				httpapi.Error(c, http.StatusInternalServerError, err.Error())
				return
			}
			c.Redirect(http.StatusFound, u)
		}
		githubCallback := func(c *gin.Context) {
			user, rawCookie, returnTo, err := authSvc.HandleGitHubCallback(c.Request.Context(), c.Query("code"), c.Query("state"))
			if err != nil {
				var denied *auth.AccessDeniedError
				if errors.As(err, &denied) {
					deniedURL := "/app/access-denied"
					if denied.Email != "" {
						deniedURL += "?email=" + url.QueryEscape(denied.Email)
					}
					c.Redirect(http.StatusFound, deniedURL)
					return
				}
				httpapi.LogWarnRequest(c, "auth_callback_failed",
					"path", c.Request.URL.Path,
					"client_ip", c.ClientIP(),
					"error", err.Error(),
				)
				httpapi.Error(c, http.StatusUnauthorized, err.Error())
				return
			}
			setSessionCookie(c, cfg, "cop_app_session", rawCookie, cfg.AppSessionTTL)
			_ = user
			c.Redirect(http.StatusFound, returnTo)
		}
		api.GET("/auth/github/login", githubLogin)
		api.GET("/auth/github/callback", githubCallback)
	}
	api.GET("/auth/me", func(c *gin.Context) {
		raw, err := c.Cookie("cop_app_session")
		if err != nil {
			httpapi.AbortUnauthorized(c)
			return
		}
		user, err := authSvc.Me(c.Request.Context(), raw)
		if err != nil || user == nil {
			httpapi.AbortUnauthorized(c)
			return
		}
		httpapi.JSON(c, http.StatusOK, user)
	})
	api.POST("/auth/logout", func(c *gin.Context) {
		raw, _ := c.Cookie("cop_app_session")
		if raw != "" {
			_ = authSvc.Logout(c.Request.Context(), raw)
		}
		clearSessionCookie(c, cfg, "cop_app_session")
		httpapi.JSON(c, http.StatusOK, gin.H{"success": true})
	})
}

func registerCLIRoutes(api *gin.RouterGroup, cfg Config, authSvc authRouteService, appSvc *appuser.Service, cliSvc *clisession.Service) {
	if cliSvc == nil || authSvc == nil {
		return
	}
	api.POST("/cli/login/start", func(c *gin.Context) {
		var req clisession.StartRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		resp, err := cliSvc.Start(c.Request.Context(), req)
		if err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, resp)
	})
	api.GET("/cli/login/verify", func(c *gin.Context) {
		raw, err := c.Cookie("cop_app_session")
		if err != nil || raw == "" {
			values := url.Values{}
			values.Set("return_to", c.Request.URL.RequestURI())
			c.Redirect(http.StatusFound, "/api/auth/oauth2/login?"+values.Encode())
			return
		}
		user, err := authSvc.Me(c.Request.Context(), raw)
		if err != nil || user == nil {
			httpapi.AbortUnauthorized(c)
			return
		}
		if err := cliSvc.VerifyUserCode(c.Request.Context(), c.Query("user_code"), user.ID); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, "OfficeCLI login complete. You can return to the terminal.")
	})
	api.POST("/cli/login/poll", func(c *gin.Context) {
		var req struct {
			ChallengeID string `json:"challenge_id"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		resp, err := cliSvc.Poll(c.Request.Context(), req.ChallengeID)
		if err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, resp)
	})
	api.GET("/cli/login/complete", func(c *gin.Context) {
		raw, err := c.Cookie("cop_app_session")
		if err != nil || raw == "" {
			values := url.Values{}
			values.Set("return_to", c.Request.URL.RequestURI())
			c.Redirect(http.StatusFound, "/api/auth/oauth2/login?"+values.Encode())
			return
		}
		user, err := authSvc.Me(c.Request.Context(), raw)
		if err != nil || user == nil {
			httpapi.AbortUnauthorized(c)
			return
		}
		redirectTo, _, err := cliSvc.Complete(c.Request.Context(), c.Query("challenge_id"), user.ID)
		if err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		c.Redirect(http.StatusFound, redirectTo)
	})
	api.POST("/cli/login/exchange", func(c *gin.Context) {
		var req clisession.ExchangeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		resp, err := cliSvc.Exchange(c.Request.Context(), req)
		if err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		if resp != nil && resp.UserID != 0 && strings.TrimSpace(req.FingerprintHash) != "" && appSvc != nil {
			// Best-effort: merge anonymous credits from this fingerprint into the
			// user account. Failure is non-fatal — login itself already succeeded.
			_, _ = appSvc.MergeAnonymousCreditsIntoUser(c.Request.Context(), req.FingerprintHash, resp.UserID)
		}
		httpapi.JSON(c, http.StatusOK, resp)
	})
	api.GET("/cli/session", func(c *gin.Context) {
		resp, err := cliSvc.Session(c.Request.Context(), bearerToken(c.GetHeader("Authorization")))
		if err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, resp)
	})
	api.POST("/cli/logout", func(c *gin.Context) {
		if err := cliSvc.Logout(c.Request.Context(), bearerToken(c.GetHeader("Authorization"))); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, gin.H{"success": true})
	})
	_ = cfg
}

func registerAdminRoutes(api *gin.RouterGroup, cfg Config, adminSvc adminRouteService) {
	adminLoginRatePerMinute := cfg.AdminLoginRateLimitPerMinute
	if adminLoginRatePerMinute <= 0 {
		adminLoginRatePerMinute = defaultAdminLoginRateLimit(cfg.AppEnv)
	}
	rateLimitTTL := cfg.RateLimitVisitorTTL
	if rateLimitTTL <= 0 {
		rateLimitTTL = defaultRateLimitVisitorTTL(cfg.AppEnv)
	}
	loginLimiter := httpapi.NewRateLimitMiddleware(
		rate.Every(time.Minute/time.Duration(adminLoginRatePerMinute)),
		adminLoginRatePerMinute,
		rateLimitTTL,
	)
	adminOAuthLogin := func(c *gin.Context) {
		returnTo := c.DefaultQuery("return_to", "/admin")
		if cfg.AdminGoogleMockEnabled && cfg.AppEnv == "development" {
			_, rawCookie, err := adminSvc.MockGoogleLogin(c.Request.Context(), cfg.AdminGoogleMockEmail, cfg.AdminGoogleMockName)
			if err != nil {
				httpapi.Error(c, http.StatusInternalServerError, err.Error())
				return
			}
			setSessionCookie(c, cfg, "cop_admin_session", rawCookie, cfg.AdminSessionTTL)
			c.Redirect(http.StatusFound, returnTo)
			return
		}
		url, err := adminSvc.LoginURL(c.Request.Context(), returnTo)
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		c.Redirect(http.StatusFound, url)
	}
	adminOAuthCallback := func(c *gin.Context) {
		_, rawCookie, returnTo, err := adminSvc.HandleGoogleCallback(c.Request.Context(), c.Query("code"), c.Query("state"))
		if err != nil {
			var denied *admin.AccessDeniedError
			if errors.As(err, &denied) {
				deniedURL := "/admin/access-denied"
				if denied.Email != "" {
					deniedURL += "?email=" + url.QueryEscape(denied.Email)
				}
				c.Redirect(http.StatusFound, deniedURL)
				return
			}
			httpapi.LogWarnRequest(c, "admin_google_callback_failed",
				"path", c.Request.URL.Path,
				"client_ip", c.ClientIP(),
				"error", err.Error(),
			)
			httpapi.Error(c, http.StatusUnauthorized, err.Error())
			return
		}
		setSessionCookie(c, cfg, "cop_admin_session", rawCookie, cfg.AdminSessionTTL)
		c.Redirect(http.StatusFound, returnTo)
	}
	api.GET("/admin/auth/oauth2/login", adminOAuthLogin)
	api.GET("/admin/auth/oauth2/callback", adminOAuthCallback)
	api.GET("/admin/auth/google/login", adminOAuthLogin)
	api.GET("/admin/auth/google/callback", adminOAuthCallback)
	api.POST("/admin/login", loginLimiter, func(c *gin.Context) {
		var req admin.LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		cookieValue, err := adminSvc.Login(c.Request.Context(), req.Password)
		if err != nil {
			httpapi.LogWarnRequest(c, "admin_login_failed",
				"path", c.Request.URL.Path,
				"client_ip", c.ClientIP(),
				"error", err.Error(),
			)
			httpapi.Error(c, http.StatusUnauthorized, err.Error())
			return
		}
		setSessionCookie(c, cfg, "cop_admin_session", cookieValue, cfg.AdminSessionTTL)
		httpapi.JSON(c, http.StatusOK, admin.LoginResponse{Success: true})
	})
	api.GET("/admin/session", func(c *gin.Context) {
		raw, err := c.Cookie("cop_admin_session")
		if err != nil {
			httpapi.AbortUnauthorized(c)
			return
		}
		identity, err := adminSvc.CurrentIdentity(c.Request.Context(), raw)
		if err != nil || identity == nil {
			httpapi.AbortUnauthorized(c)
			return
		}
		httpapi.JSON(c, http.StatusOK, identity)
	})
	api.POST("/admin/logout", func(c *gin.Context) {
		raw, _ := c.Cookie("cop_admin_session")
		if raw != "" {
			_ = adminSvc.Logout(c.Request.Context(), raw)
		}
		clearSessionCookie(c, cfg, "cop_admin_session")
		httpapi.JSON(c, http.StatusOK, gin.H{"success": true})
	})

	protected := api.Group("/admin")
	protected.Use(httpapi.RequireAdmin(adminSvc.ResolveSession, "cop_admin_session"))
	protected.GET("/overview", func(c *gin.Context) {
		data, err := adminSvc.Overview(c.Request.Context())
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.GET("/fingerprint-quality", func(c *gin.Context) {
		data, err := adminSvc.FingerprintQuality(c.Request.Context())
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.GET("/operations/funnel", func(c *gin.Context) {
		now := time.Now().UTC()
		var windowStart time.Time
		switch c.DefaultQuery("range", "30d") {
		case "24h":
			windowStart = now.Add(-24 * time.Hour)
		case "7d":
			windowStart = now.AddDate(0, 0, -7)
		case "30d":
			windowStart = now.AddDate(0, 0, -30)
		default:
			httpapi.Error(c, http.StatusBadRequest, "range must be 24h, 7d, or 30d")
			return
		}
		data, err := adminSvc.OperationsFunnel(c.Request.Context(), windowStart, now)
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.GET("/api-keys", func(c *gin.Context) {
		var ownerUserID *uint64
		if raw := c.Query("user_id"); raw != "" {
			parsed, _ := strconv.ParseUint(raw, 10, 64)
			if parsed > 0 {
				ownerUserID = &parsed
			}
		}
		data, err := adminSvc.ListAPIKeys(c.Request.Context(), ownerUserID)
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.GET("/api-keys/:id/plaintext", func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		actor := "admin"
		if raw, err := c.Cookie("cop_admin_session"); err == nil && raw != "" {
			if identity, err := adminSvc.CurrentIdentity(c.Request.Context(), raw); err == nil && identity != nil && strings.TrimSpace(identity.Email) != "" {
				actor = identity.Email
			}
		}
		plaintext, err := adminSvc.GetAPIKeyPlaintext(c.Request.Context(), id, actor)
		switch {
		case err == nil:
			httpapi.JSON(c, http.StatusOK, admin.APIKeyPlaintextResponse{PlaintextKey: plaintext})
		case errors.Is(err, apikey.ErrAPIKeyNotFound):
			httpapi.Error(c, http.StatusNotFound, err.Error())
		case errors.Is(err, apikey.ErrPlaintextUnavailable):
			httpapi.Error(c, http.StatusConflict, err.Error())
		default:
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
		}
	})
	protected.POST("/api-keys", func(c *gin.Context) {
		var req admin.CreateAPIKeyRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		data, _, err := adminSvc.CreateAPIKey(c.Request.Context(), req)
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.PATCH("/api-keys/:id", func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var req admin.UpdateAPIKeyRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		if err := adminSvc.UpdateAPIKey(c.Request.Context(), id, req); err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, gin.H{"success": true})
	})
	protected.GET("/usage-events", func(c *gin.Context) {
		filter := sqlstore.UsageEventFilter{
			Mode:        c.Query("mode"),
			Result:      c.Query("result"),
			ReasonCode:  c.Query("reason_code"),
			Fingerprint: c.Query("fingerprint_hash"),
			ClientIP:    c.Query("client_ip"),
			RequestID:   c.Query("request_id"),
		}
		if raw := c.Query("api_key_id"); raw != "" {
			parsed, _ := strconv.ParseUint(raw, 10, 64)
			filter.APIKeyID = &parsed
		}
		if raw := c.Query("user_id"); raw != "" {
			parsed, _ := strconv.ParseUint(raw, 10, 64)
			filter.UserID = &parsed
		}
		if raw := c.Query("start_time"); raw != "" {
			if ts, err := time.Parse(time.RFC3339, raw); err == nil {
				filter.StartTime = &ts
			}
		}
		if raw := c.Query("end_time"); raw != "" {
			if ts, err := time.Parse(time.RFC3339, raw); err == nil {
				filter.EndTime = &ts
			}
		}
		data, err := adminSvc.ListUsageEvents(c.Request.Context(), filter)
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.GET("/preferences/:page_key", func(c *gin.Context) {
		adminEmail, ok := currentAdminEmail(c, adminSvc)
		if !ok {
			httpapi.Error(c, http.StatusBadRequest, "admin email is required")
			return
		}
		preference, err := adminSvc.GetPreference(c.Request.Context(), adminEmail, c.Param("page_key"))
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		if preference == nil || strings.TrimSpace(preference.PreferencesJSON) == "" {
			httpapi.JSON(c, http.StatusOK, gin.H{})
			return
		}
		httpapi.JSON(c, http.StatusOK, json.RawMessage(preference.PreferencesJSON))
	})
	protected.PUT("/preferences/:page_key", func(c *gin.Context) {
		adminEmail, ok := currentAdminEmail(c, adminSvc)
		if !ok {
			httpapi.Error(c, http.StatusBadRequest, "admin email is required")
			return
		}
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		var payload any
		if err := json.Unmarshal(body, &payload); err != nil {
			httpapi.Error(c, http.StatusBadRequest, "preferences must be valid JSON")
			return
		}
		normalized, err := json.Marshal(payload)
		if err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		preference, err := adminSvc.SavePreference(c.Request.Context(), adminEmail, c.Param("page_key"), string(normalized))
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, json.RawMessage(preference.PreferencesJSON))
	})
	protected.GET("/users", func(c *gin.Context) {
		data, err := adminSvc.ListUsers(c.Request.Context(), c.Query("query"))
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.PATCH("/users/:id", func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var req admin.UpdateUserRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		if err := adminSvc.UpdateUser(c.Request.Context(), id, req); err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, gin.H{"success": true})
	})
	protected.GET("/orders", func(c *gin.Context) {
		data, err := adminSvc.ListOrders(c.Request.Context())
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.PATCH("/orders/:id", func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var req admin.UpdateOrderRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		if err := adminSvc.UpdateOrder(c.Request.Context(), id, req); err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, gin.H{"success": true})
	})
	protected.GET("/billing-events", func(c *gin.Context) {
		data, err := adminSvc.ListBillingEvents(c.Request.Context())
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.GET("/credit-ledger", func(c *gin.Context) {
		includeZeroDelta := c.Query("include_zero_delta") == "true"
		data, err := adminSvc.ListCreditLedger(c.Request.Context(), includeZeroDelta)
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.GET("/growth", func(c *gin.Context) {
		data, err := adminSvc.Growth(c.Request.Context())
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.GET("/quota-sources", func(c *gin.Context) {
		filter := admin.QuotaSourcesFilter{
			Fingerprint: c.Query("fingerprint"),
			UsageDate:   c.Query("usage_date"),
			KeyPrefix:   c.Query("key_prefix"),
		}
		if raw := c.Query("user_id"); raw != "" {
			filter.UserID, _ = strconv.ParseUint(raw, 10, 64)
		}
		data, err := adminSvc.QuotaSources(c.Request.Context(), filter)
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.GET("/hosted-pricing-rules", func(c *gin.Context) {
		data, err := adminSvc.HostedPricingRules(c.Request.Context())
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.GET("/hosted-billing", func(c *gin.Context) {
		data, err := adminSvc.HostedBillingConfig(c.Request.Context())
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.PATCH("/hosted-pricing-settings", func(c *gin.Context) {
		var req admin.UpdateHostedPricingSettingsRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		data, err := adminSvc.UpdateHostedPricingSettings(c.Request.Context(), req)
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.POST("/hosted-model-pricing-configs", func(c *gin.Context) {
		var req admin.UpsertHostedModelPricingConfigRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		data, err := adminSvc.CreateHostedModelPricingConfig(c.Request.Context(), req)
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.PATCH("/hosted-model-pricing-configs/:id", func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var req admin.UpsertHostedModelPricingConfigRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		data, err := adminSvc.UpdateHostedModelPricingConfig(c.Request.Context(), id, req)
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.POST("/hosted-pricing-rules", func(c *gin.Context) {
		var req admin.UpsertHostedPricingRuleRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		data, err := adminSvc.CreateHostedPricingRule(c.Request.Context(), req)
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.PATCH("/hosted-pricing-rules/:id", func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var req admin.UpsertHostedPricingRuleRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		data, err := adminSvc.UpdateHostedPricingRule(c.Request.Context(), id, req)
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.POST("/hosted-credit-packs", func(c *gin.Context) {
		var req admin.UpsertHostedCreditPackRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		data, err := adminSvc.CreateHostedCreditPack(c.Request.Context(), req)
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.PATCH("/hosted-credit-packs/:id", func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var req admin.UpsertHostedCreditPackRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		data, err := adminSvc.UpdateHostedCreditPack(c.Request.Context(), id, req)
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})

	protected.GET("/redemption-codes", func(c *gin.Context) {
		var req admin.ListRedemptionCodesRequest
		if err := c.ShouldBindQuery(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		data, err := adminSvc.ListRedemptionCodes(c.Request.Context(), req)
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.POST("/redemption-codes", func(c *gin.Context) {
		var req admin.CreateRedemptionCodeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		actor, _ := currentAdminEmail(c, adminSvc)
		data, err := adminSvc.CreateRedemptionCode(c.Request.Context(), actor, req)
		if err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.GET("/redemption-codes/redemptions", func(c *gin.Context) {
		var req admin.ListRedemptionRecordsRequest
		if err := c.ShouldBindQuery(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		data, err := adminSvc.ListRedemptionRecords(c.Request.Context(), req)
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.GET("/redemption-codes/:id", func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		data, err := adminSvc.GetRedemptionCode(c.Request.Context(), id)
		if err != nil {
			httpapi.Error(c, http.StatusNotFound, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.PATCH("/redemption-codes/:id", func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var req admin.UpdateRedemptionCodeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		actor, _ := currentAdminEmail(c, adminSvc)
		data, err := adminSvc.UpdateRedemptionCode(c.Request.Context(), actor, id, req)
		if err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.POST("/redemption-codes/:id/enable", func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		actor, _ := currentAdminEmail(c, adminSvc)
		data, err := adminSvc.EnableRedemptionCode(c.Request.Context(), actor, id)
		if err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.POST("/redemption-codes/:id/disable", func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		actor, _ := currentAdminEmail(c, adminSvc)
		data, err := adminSvc.DisableRedemptionCode(c.Request.Context(), actor, id)
		if err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
}

func currentAdminEmail(c *gin.Context, adminSvc adminRouteService) (string, bool) {
	raw, err := c.Cookie("cop_admin_session")
	if err != nil || raw == "" {
		return "", false
	}
	identity, err := adminSvc.CurrentIdentity(c.Request.Context(), raw)
	if err != nil || identity == nil {
		return "", false
	}
	email := strings.ToLower(strings.TrimSpace(identity.Email))
	return email, email != ""
}

func registerPreviewRoutes(r *egin.Component, cfg Config, authSvc authRouteService, shares *previewshare.Service, sdkHandler *officesdk.Handler, sdkProvider *officesdk.FileProvider) {
	if shares == nil || sdkHandler == nil || sdkProvider == nil {
		return
	}
	r.GET("/p/:shareToken", func(c *gin.Context) {
		share, status, err := shares.ValidateEntryRequest(c.Request.Context(), c.Param("shareToken"))
		if err != nil {
			httpapi.Error(c, status, err.Error())
			return
		}
		if shares.HasAccessCookie(c, share) {
			servePreviewShare(c, share, sdkHandler, sdkProvider)
			return
		}
		if !hasPreviewLogin(c, authSvc) {
			c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(previewshare.RenderLoginPage(share, previewLoginURL(cfg, c.Request), "")))
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(previewshare.RenderPasswordPage(share, "")))
	})
	r.POST("/p/:shareToken", func(c *gin.Context) {
		share, status, err := shares.ValidateEntryRequest(c.Request.Context(), c.Param("shareToken"))
		if err != nil {
			httpapi.Error(c, status, err.Error())
			return
		}
		if !hasPreviewLogin(c, authSvc) {
			c.Data(http.StatusUnauthorized, "text/html; charset=utf-8", []byte(previewshare.RenderLoginPage(share, previewLoginURL(cfg, c.Request), "Sign in is required before opening this preview.")))
			return
		}
		password := strings.TrimSpace(c.PostForm("password"))
		if password == "" {
			var body struct {
				Password string `json:"password"`
			}
			if bindErr := c.ShouldBindJSON(&body); bindErr == nil {
				password = strings.TrimSpace(body.Password)
			}
		}
		if !shares.VerifyPassword(share, password) {
			c.Data(http.StatusUnauthorized, "text/html; charset=utf-8", []byte(previewshare.RenderPasswordPage(share, "Incorrect password. Please try again.")))
			return
		}
		shares.IssueAccessCookie(c, share)
		servePreviewShare(c, share, sdkHandler, sdkProvider)
	})
	r.GET("/p/:shareToken/raw", func(c *gin.Context) {
		share, status, err := shares.ValidateEntryRequest(c.Request.Context(), c.Param("shareToken"))
		if err != nil {
			httpapi.Error(c, status, err.Error())
			return
		}
		if !strings.EqualFold(strings.TrimSpace(share.FileType), "img") {
			httpapi.Error(c, http.StatusNotFound, "image preview not found")
			return
		}
		if !shares.HasAccessCookie(c, share) {
			httpapi.Error(c, http.StatusUnauthorized, "preview access is required")
			return
		}
		serveImageRaw(c, share, sdkProvider)
	})

	osdk := r.Group("/officesdk")
	osdk.GET("/page", sdkHandler.ServePage)
	osdk.HEAD("/page", sdkHandler.ServePage)
	osdk.GET("/sdk-params", sdkHandler.GetSDKParams)
	osdk.HEAD("/sdk-params", sdkHandler.GetSDKParams)
	osdk.PUT("/storage/upload", sdkProvider.HandleProxyUpload)
	osdk.GET("/proxy/download", sdkProvider.HandleProxyDownload)
	osdk.HEAD("/proxy/download", sdkProvider.HandleProxyDownload)

	sdkoffice.NewServer(sdkoffice.Config{
		FileProvider: sdkProvider,
		AIProvider:   officesdk.NewNoopAIProvider(),
		Prefix:       "",
	}, r.Engine)

	registerOfficeSDKProxy(r.Engine, cfg)
}

func servePreviewShare(c *gin.Context, share *previewshare.PreviewShare, sdkHandler *officesdk.Handler, sdkProvider *officesdk.FileProvider) {
	if share != nil && strings.EqualFold(strings.TrimSpace(share.FileType), "report") {
		serveReportPreview(c, share, sdkProvider)
		return
	}
	if share != nil && strings.EqualFold(strings.TrimSpace(share.FileType), "img") {
		serveImagePreview(c, share)
		return
	}
	sdkHandler.ServePageForFile(c, share.FileID)
}

func serveImagePreview(c *gin.Context, share *previewshare.PreviewShare) {
	if c == nil || share == nil {
		httpapi.Error(c, http.StatusInternalServerError, "image preview unavailable")
		return
	}
	rawURL := "/p/" + url.PathEscape(share.ShareToken) + "/raw"
	downloadURL := rawURL + "?download=1"
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(previewshare.RenderImagePage(share, rawURL, downloadURL)))
}

func serveImageRaw(c *gin.Context, share *previewshare.PreviewShare, sdkProvider *officesdk.FileProvider) {
	if c == nil || share == nil || sdkProvider == nil {
		httpapi.Error(c, http.StatusInternalServerError, "image preview unavailable")
		return
	}
	imageBytes, err := sdkProvider.ReadObject(c.Request.Context(), share.StorageKey)
	if err != nil {
		httpapi.Error(c, http.StatusNotFound, "image preview not found")
		return
	}
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(share.FileName)))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if strings.TrimSpace(c.Query("download")) == "1" {
		name := strings.ReplaceAll(share.FileName, "\"", "")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", name))
	} else {
		c.Header("Content-Disposition", "inline")
	}
	c.Header("Cache-Control", "private, max-age=60")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, contentType, imageBytes)
}

func serveReportPreview(c *gin.Context, share *previewshare.PreviewShare, sdkProvider *officesdk.FileProvider) {
	if c == nil || share == nil || sdkProvider == nil {
		httpapi.Error(c, http.StatusInternalServerError, "report preview unavailable")
		return
	}
	reportHTML, err := sdkProvider.ReadObject(c.Request.Context(), share.StorageKey)
	if err != nil {
		httpapi.Error(c, http.StatusNotFound, "report preview not found")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(previewshare.RenderReportPage(share, reportHTML)))
}

func previewLoginURL(cfg Config, r *http.Request) string {
	currentURL := currentRequestURL(r)
	values := url.Values{}
	values.Set("return_to", currentURL)
	return joinURL(cfg.PlatformBaseURL, "/api/auth/oauth2/login?"+values.Encode())
}

func currentRequestURL(r *http.Request) string {
	base := officesdkRequestBaseURL(r)
	if base == "" {
		return ""
	}
	if r == nil || r.URL == nil {
		return base
	}
	return base + r.URL.RequestURI()
}

func hasPreviewLogin(c *gin.Context, authSvc authRouteService) bool {
	if authSvc == nil {
		return true
	}
	if c == nil {
		return false
	}
	raw, err := c.Cookie("cop_app_session")
	if err != nil || strings.TrimSpace(raw) == "" {
		return false
	}
	_, err = authSvc.Me(c.Request.Context(), raw)
	return err == nil
}

func officesdkRequestBaseURL(r *http.Request) string {
	if r == nil {
		return ""
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	if host == "" {
		return ""
	}
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	return scheme + "://" + host
}

func registerOfficeSDKProxy(router *gin.Engine, cfg Config) {
	target := strings.TrimSpace(cfg.OfficeSDKHost)
	if target == "" {
		return
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			publicHost := forwardedRequestHost(pr.In)
			publicProto := forwardedRequestProto(pr.In)
			pr.SetURL(parsed)
			if publicHost != "" {
				pr.Out.Host = publicHost
				pr.Out.Header.Set("X-Forwarded-Host", publicHost)
			}
			if publicProto != "" {
				pr.Out.Header.Set("X-Forwarded-Proto", publicProto)
				pr.Out.Header.Set("X-Forwarded-Port", forwardedRequestPort(publicHost, publicProto))
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			rewriteOfficeSDKProxyLocation(resp)
			return nil
		},
	}
	router.Any("/sdk/turbo-ai/*path", func(c *gin.Context) {
		proxy.ServeHTTP(c.Writer, c.Request)
	})
}

func forwardedRequestHost(r *http.Request) string {
	if r == nil {
		return ""
	}
	if host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); host != "" {
		return host
	}
	return strings.TrimSpace(r.Host)
}

func forwardedRequestProto(r *http.Request) string {
	if r == nil {
		return ""
	}
	if scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); scheme != "" {
		return scheme
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func forwardedRequestPort(host, proto string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		if strings.EqualFold(strings.TrimSpace(proto), "https") {
			return "443"
		}
		return "80"
	}
	if _, port, err := net.SplitHostPort(host); err == nil && strings.TrimSpace(port) != "" {
		return port
	}
	if strings.EqualFold(strings.TrimSpace(proto), "https") {
		return "443"
	}
	return "80"
}

func rewriteOfficeSDKProxyLocation(resp *http.Response) {
	if resp == nil || resp.Request == nil {
		return
	}
	location := strings.TrimSpace(resp.Header.Get("Location"))
	if location == "" {
		return
	}
	parsedLocation, err := url.Parse(location)
	if err != nil || !parsedLocation.IsAbs() {
		return
	}
	publicBase := officesdkRequestBaseURL(resp.Request)
	if publicBase == "" {
		return
	}
	parsedPublicBase, err := url.Parse(publicBase)
	if err != nil || parsedPublicBase.Host == "" {
		return
	}
	if strings.EqualFold(parsedLocation.Scheme, parsedPublicBase.Scheme) && strings.EqualFold(parsedLocation.Host, parsedPublicBase.Host) {
		return
	}
	parsedLocation.Scheme = parsedPublicBase.Scheme
	parsedLocation.Host = parsedPublicBase.Host
	resp.Header.Set("Location", parsedLocation.String())
}

func setSessionCookie(c *gin.Context, cfg Config, name, value string, ttl time.Duration) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(name, value, int(ttl.Seconds()), "/", sessionCookieDomain(cfg, name), shouldUseSecureCookies(cfg), true)
}

func clearSessionCookie(c *gin.Context, cfg Config, name string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(name, "", -1, "/", sessionCookieDomain(cfg, name), shouldUseSecureCookies(cfg), true)
}

func sessionCookieDomain(cfg Config, name string) string {
	if name == "cop_app_session" {
		return strings.TrimSpace(cfg.AppSessionCookieDomain)
	}
	return ""
}

func shouldUseSecureCookies(cfg Config) bool {
	return cfg.AppEnv == "production"
}

func bearerToken(header string) string {
	return httpapi.BearerToken(header)
}

// redemptionErrorStatus maps redemption package errors to HTTP status codes
// and stable machine-readable error codes returned to the client.
func redemptionErrorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, redemptionsvc.ErrCodeNotFound):
		return http.StatusNotFound, "code_not_found"
	case errors.Is(err, redemptionsvc.ErrCodeDisabled):
		return http.StatusForbidden, "code_disabled"
	case errors.Is(err, redemptionsvc.ErrCodeExpired):
		return http.StatusGone, "code_expired"
	case errors.Is(err, redemptionsvc.ErrCodeExhausted):
		return http.StatusGone, "code_exhausted"
	case errors.Is(err, redemptionsvc.ErrCodeAlreadyClaimed):
		return http.StatusConflict, "code_already_claimed"
	case errors.Is(err, redemptionsvc.ErrCodeRequired):
		return http.StatusBadRequest, "code_required"
	}
	return http.StatusInternalServerError, "internal_error"
}

// registerRedemptionUserRoutes mounts both /api/app/redemption-codes/redeem
// (cookie-session auth) and /api/cli/redemption-codes/redeem (Bearer auth)
// so the same business service powers all three downstream entry points
// (web app, CLI binary, desktop app, TUI).
func registerRedemptionUserRoutes(api *gin.RouterGroup, redemptionSvc *redemptionsvc.Service, authSvc *auth.Service, cliSvc *clisession.Service) {
	if redemptionSvc == nil {
		return
	}
	type redeemBody struct {
		Code   string `json:"code"`
		Source string `json:"source,omitempty"`
	}
	respond := func(c *gin.Context, resp *redemptionsvc.RedeemResponse, err error) {
		if err != nil {
			status, code := redemptionErrorStatus(err)
			httpapi.JSON(c, status, gin.H{"error": gin.H{"code": code, "message": err.Error()}})
			return
		}
		httpapi.JSON(c, http.StatusOK, resp)
	}

	if authSvc != nil {
		appGroup := api.Group("/app")
		resolver := func(cookieValue string) (uint64, string, error) {
			payload, err := authSvc.ResolveSession(cookieValue)
			if err != nil || payload == nil {
				return 0, "", err
			}
			return payload.UserID, payload.SessionID, nil
		}
		appGroup.Use(httpapi.RequireAppUser(resolver, "cop_app_session"))
		appGroup.POST("/redemption-codes/redeem", func(c *gin.Context) {
			var body redeemBody
			if err := c.ShouldBindJSON(&body); err != nil {
				httpapi.Error(c, http.StatusBadRequest, err.Error())
				return
			}
			source := strings.TrimSpace(body.Source)
			if source == "" {
				source = string(model.RedemptionSourceApp)
			}
			resp, err := redemptionSvc.Redeem(c.Request.Context(), redemptionsvc.RedeemRequest{
				UserID:    currentUserID(c),
				Code:      body.Code,
				Source:    source,
				ClientIP:  c.ClientIP(),
				UserAgent: c.Request.UserAgent(),
			})
			respond(c, resp, err)
		})
		appGroup.GET("/redemption-codes/my", func(c *gin.Context) {
			items, total, err := redemptionSvc.ListRedemptions(c.Request.Context(), redemptionsvc.ListRecordsRequest{
				UserID: currentUserID(c),
				Limit:  100,
			})
			if err != nil {
				httpapi.Error(c, http.StatusInternalServerError, err.Error())
				return
			}
			if items == nil {
				items = []model.RedemptionCodeRedemption{}
			}
			httpapi.JSON(c, http.StatusOK, gin.H{"items": items, "total": total})
		})
	}

	if cliSvc != nil {
		api.POST("/cli/redemption-codes/redeem", func(c *gin.Context) {
			session, err := cliSvc.Resolve(c.Request.Context(), bearerToken(c.GetHeader("Authorization")))
			if err != nil || session == nil || session.UserID == 0 {
				httpapi.AbortUnauthorized(c)
				return
			}
			var body redeemBody
			if err := c.ShouldBindJSON(&body); err != nil {
				httpapi.Error(c, http.StatusBadRequest, err.Error())
				return
			}
			source := strings.TrimSpace(body.Source)
			if source == "" {
				source = c.GetHeader("X-Officecli-Client")
			}
			if source == "" {
				source = string(model.RedemptionSourceCLI)
			}
			normalized := model.RedemptionSource(strings.ToLower(strings.TrimSpace(source)))
			if !normalized.Valid() {
				normalized = model.RedemptionSourceCLI
			}
			resp, err := redemptionSvc.Redeem(c.Request.Context(), redemptionsvc.RedeemRequest{
				UserID:    session.UserID,
				Code:      body.Code,
				Source:    string(normalized),
				ClientIP:  c.ClientIP(),
				UserAgent: c.Request.UserAgent(),
			})
			respond(c, resp, err)
		})
	}
}

func registerAppRoutes(api *gin.RouterGroup, cfg Config, authSvc *auth.Service, appSvc *appuser.Service, billingSvc *billing.Service, discordSvc discordOAuthRouteService, cliSvc *clisession.Service) {
	resolver := func(cookieValue string) (uint64, string, error) {
		payload, err := authSvc.ResolveSession(cookieValue)
		if err != nil || payload == nil {
			return 0, "", err
		}
		return payload.UserID, payload.SessionID, nil
	}
	protected := api.Group("/app")
	protected.Use(httpapi.RequireAppUser(resolver, "cop_app_session"))
	protected.GET("/discord/login", func(c *gin.Context) {
		returnTo := c.DefaultQuery("return_to", "/app")
		url, err := discordSvc.LoginURL(c.Request.Context(), currentUserID(c), returnTo)
		if err != nil {
			c.Redirect(http.StatusFound, appendDiscordStatusQuery(returnTo, "oauth_not_configured", err.Error()))
			return
		}
		c.Redirect(http.StatusFound, url)
	})
	api.GET("/app/discord/callback", func(c *gin.Context) {
		result, err := discordSvc.HandleCallback(c.Request.Context(), c.Query("code"), c.Query("state"))
		if err != nil {
			c.Redirect(http.StatusFound, appendDiscordStatusQuery("/app", "error", err.Error()))
			return
		}
		status := result.VerificationStatus
		if status == "" {
			status = "connected"
		}
		redirectTo := appendDiscordStatusQuery(result.ReturnTo, status, result.VerificationBlockedReason)
		if result.RewardGranted {
			redirectTo = appendDiscordRewardGrantedQuery(redirectTo)
		}
		c.Redirect(http.StatusFound, redirectTo)
	})
	protected.GET("/overview", func(c *gin.Context) {
		userID := currentUserID(c)
		data, err := appSvc.Overview(c.Request.Context(), userID)
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.GET("/quota-summary", func(c *gin.Context) {
		data, err := appSvc.QuotaSummary(c.Request.Context(), currentUserID(c))
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.GET("/growth", func(c *gin.Context) {
		data, err := appSvc.Growth(c.Request.Context(), currentUserID(c))
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.GET("/discord/status", func(c *gin.Context) {
		data, err := appSvc.DiscordStatus(c.Request.Context(), currentUserID(c))
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.GET("/cli-sessions", func(c *gin.Context) {
		if cliSvc == nil {
			httpapi.Error(c, http.StatusServiceUnavailable, "cli sessions are unavailable")
			return
		}
		sessions, err := cliSvc.StoreSessions(c.Request.Context(), currentUserID(c))
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, sessions)
	})
	protected.DELETE("/cli-sessions/:id", func(c *gin.Context) {
		if cliSvc == nil {
			httpapi.Error(c, http.StatusServiceUnavailable, "cli sessions are unavailable")
			return
		}
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			httpapi.Error(c, http.StatusBadRequest, "invalid cli session id")
			return
		}
		if err := cliSvc.RevokeUserSession(c.Request.Context(), currentUserID(c), id); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, gin.H{"success": true})
	})
	protected.POST("/discord/connect", func(c *gin.Context) {
		var req appuser.ConnectDiscordRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		data, err := appSvc.ConnectDiscord(c.Request.Context(), currentUserID(c), req)
		if err != nil {
			httpapi.Error(c, appDiscordStatusCode(err), err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.GET("/api-keys", func(c *gin.Context) {
		data, err := appSvc.ListAPIKeys(c.Request.Context(), currentUserID(c))
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.GET("/api-keys/:id/plaintext", func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		plaintext, err := appSvc.GetAPIKeyPlaintext(c.Request.Context(), currentUserID(c), id)
		switch {
		case err == nil:
			httpapi.JSON(c, http.StatusOK, appuser.APIKeyPlaintextResponse{PlaintextKey: plaintext})
		case errors.Is(err, apikey.ErrAPIKeyNotFound):
			httpapi.Error(c, http.StatusNotFound, err.Error())
		case errors.Is(err, apikey.ErrPlaintextUnavailable):
			httpapi.Error(c, http.StatusConflict, err.Error())
		default:
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
		}
	})
	protected.POST("/api-keys", func(c *gin.Context) {
		var req appuser.CreateAPIKeyRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		data, err := appSvc.CreateAPIKey(c.Request.Context(), currentUserID(c), req)
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.PATCH("/api-keys/:id", func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var req appuser.UpdateAPIKeyRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		if err := appSvc.UpdateAPIKey(c.Request.Context(), currentUserID(c), id, req); err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, gin.H{"success": true})
	})
	protected.GET("/usage-events", func(c *gin.Context) {
		data, err := appSvc.ListUsageEvents(c.Request.Context(), currentUserID(c))
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.GET("/credit-ledger", func(c *gin.Context) {
		data, err := appSvc.ListCreditLedger(c.Request.Context(), currentUserID(c))
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.GET("/orders", func(c *gin.Context) {
		if cfg.AppMockDataEnabled && cfg.AppEnv == "development" {
			httpapi.JSON(c, http.StatusOK, appuser.MockOrders())
			return
		}
		data, err := billingSvc.ListOrdersByUser(c.Request.Context(), currentUserID(c))
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		filtered := make([]model.Order, 0, len(data))
		for _, order := range data {
			if order.PackKind == model.PackKindExternalGeneration {
				continue
			}
			filtered = append(filtered, order)
		}
		httpapi.JSON(c, http.StatusOK, filtered)
	})
	protected.POST("/orders/reconcile", func(c *gin.Context) {
		var req billing.ReconcileOrderRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		req.UserID = currentUserID(c)
		order, err := billingSvc.ReconcileCheckoutSession(c.Request.Context(), req)
		switch {
		case err == nil:
			awaiting := order != nil && order.Status == model.OrderStatusPending
			httpapi.JSON(c, http.StatusOK, gin.H{
				"order":                 order,
				"awaiting_confirmation": awaiting,
			})
		case errors.Is(err, billing.ErrCheckoutSessionIDRequired):
			httpapi.Error(c, http.StatusBadRequest, err.Error())
		case errors.Is(err, billing.ErrOrderNotFound), errors.Is(err, billing.ErrOrderForbidden):
			httpapi.Error(c, http.StatusNotFound, err.Error())
		default:
			httpapi.Error(c, http.StatusBadGateway, err.Error())
		}
	})
	protected.GET("/pricing", func(c *gin.Context) {
		httpapi.JSON(c, http.StatusOK, billingSvc.Pricing())
	})
	protected.POST("/checkout", func(c *gin.Context) {
		var req billing.CheckoutRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		req.UserID = currentUserID(c)
		order, checkoutURL, err := billingSvc.CreateCheckout(c.Request.Context(), req)
		if err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, gin.H{"order": order, "checkout_url": checkoutURL})
	})
}

func appDiscordStatusCode(err error) int {
	switch {
	case errors.Is(err, growthsvc.ErrDiscordUserIDRequired),
		errors.Is(err, growthsvc.ErrDiscordUsernameRequired):
		return http.StatusBadRequest
	case errors.Is(err, growthsvc.ErrUserAlreadyLinked),
		errors.Is(err, growthsvc.ErrDiscordAlreadyLinked):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func appendDiscordStatusQuery(target, status, reason string) string {
	parsed, err := url.Parse(target)
	if err != nil {
		return "/app?discord=" + url.QueryEscape(status)
	}
	query := parsed.Query()
	query.Set("discord", status)
	if strings.TrimSpace(reason) != "" {
		query.Set("discord_message", reason)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func appendDiscordRewardGrantedQuery(target string) string {
	parsed, err := url.Parse(target)
	if err != nil {
		return target
	}
	query := parsed.Query()
	query.Set("discord_reward", "granted")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func registerStripeRoutes(api *gin.RouterGroup, billingSvc stripeRouteService) {
	api.POST("/stripe/webhook", func(c *gin.Context) {
		payload, err := c.GetRawData()
		if err != nil {
			httpapi.LogWarnRequest(c, "stripe_webhook_read_failed",
				"path", c.Request.URL.Path,
				"client_ip", c.ClientIP(),
				"error", err.Error(),
			)
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		if err := billingSvc.HandleWebhook(c.Request.Context(), payload, c.GetHeader("Stripe-Signature")); err != nil {
			httpapi.LogWarnRequest(c, "stripe_webhook_failed",
				"path", c.Request.URL.Path,
				"client_ip", c.ClientIP(),
				"error", err.Error(),
			)
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, gin.H{"received": true})
	})
}

func registerIssueReportRoutes(api *gin.RouterGroup, cfg Config, cliSvc *clisession.Service, issueReportsSvc *issuereports.Service) {
	if cliSvc == nil || issueReportsSvc == nil {
		return
	}

	rateLimitTTL := cfg.RateLimitVisitorTTL
	if rateLimitTTL <= 0 {
		rateLimitTTL = defaultRateLimitVisitorTTL(cfg.AppEnv)
	}

	issueReportsHandler := issuereports.NewHandler(issueReportsSvc, cliSvc)

	// Authenticated route: 60/min keyed by hashed token (avoid retaining credential
	// material in the limiter map) or IP fallback. IP-based limits assume the
	// platform-level Gin trusted-proxy allowlist is configured at the engine; if not,
	// X-Forwarded-For can rotate the bucket per request — tracked as M1b prerequisite.
	mainLimiter := httpapi.NewRateLimitMiddlewareWithKey(
		rate.Every(time.Minute/60), 60, rateLimitTTL,
		func(c *gin.Context) string {
			if tok := httpapi.BearerToken(c.GetHeader("Authorization")); tok != "" {
				sum := sha256.Sum256([]byte(tok))
				return "user:" + hex.EncodeToString(sum[:8])
			}
			return "ip:" + c.ClientIP()
		},
	)
	api.POST("/issue-reports", mainLimiter, issueReportsHandler.Authenticated)

	// Anonymous route: 30/min/IP + global 100/min.
	anonIPLimiter := httpapi.NewRateLimitMiddlewareWithKey(
		rate.Every(time.Minute/30), 30, rateLimitTTL,
		func(c *gin.Context) string { return "anon-ip:" + c.ClientIP() },
	)
	anonGlobalLimiter := httpapi.NewRateLimitMiddlewareWithKey(
		rate.Every(time.Minute/100), 100, rateLimitTTL,
		func(c *gin.Context) string { return "anon-global" },
	)
	api.POST("/issue-reports/anonymous", anonIPLimiter, anonGlobalLimiter, issueReportsHandler.Anonymous)
}

func registerStatic(router *gin.Engine, cfg Config) {
	registerStaticDir(router, "/admin/assets", cfg.AdminStaticDir, "assets")
	registerStaticDir(router, "/app/assets", cfg.AppStaticDir, "assets")
	registerStaticDir(router, "/assets", cfg.SiteStaticDir, "assets")
	registerStaticFile(router, "/favicon.svg", cfg.SiteStaticDir, "favicon.svg")
	registerStaticFile(router, "/og-cover.svg", cfg.SiteStaticDir, "og-cover.svg")
	registerStaticFile(router, "/robots.txt", cfg.SiteStaticDir, "robots.txt")
	registerStaticFile(router, "/sitemap.xml", cfg.SiteStaticDir, "sitemap.xml")

	adminDir := absIfExists(cfg.AdminStaticDir)
	appDir := absIfExists(cfg.AppStaticDir)
	siteDir := absIfExists(cfg.SiteStaticDir)

	router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") {
			httpapi.Error(c, http.StatusNotFound, "not found")
			return
		}
		switch domainMode(c.Request.Host, cfg) {
		case "platform":
			if strings.HasPrefix(path, "/admin") {
				serveIndexOr404(c, adminDir, "admin ui not built")
				return
			}
			if strings.HasPrefix(path, "/app") {
				serveIndexOr404(c, appDir, "app ui not built")
				return
			}
			c.Redirect(http.StatusFound, "/app")
			return
		case "site":
			if strings.HasPrefix(path, "/admin") || strings.HasPrefix(path, "/app") {
				c.Redirect(http.StatusFound, joinURL(cfg.PlatformBaseURL, path))
				return
			}
			serveSiteIndexOr404(c, siteDir, path, "site ui not built")
			return
		}
		switch {
		case strings.HasPrefix(path, "/admin"):
			serveIndexOr404(c, adminDir, "admin ui not built")
		case strings.HasPrefix(path, "/app"):
			serveIndexOr404(c, appDir, "app ui not built")
		default:
			serveSiteIndexOr404(c, siteDir, path, "site ui not built")
		}
	})
}

func domainMode(requestHost string, cfg Config) string {
	switch {
	case hostMatchesBaseURL(requestHost, cfg.PlatformBaseURL):
		return "platform"
	case hostMatchesBaseURL(requestHost, cfg.SiteBaseURL):
		return "site"
	default:
		return ""
	}
}

func hostMatchesBaseURL(requestHost, baseURL string) bool {
	if requestHost == "" || baseURL == "" {
		return false
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		return false
	}
	return stripPort(requestHost) == stripPort(u.Host)
}

func stripPort(host string) string {
	return strings.Split(host, ":")[0]
}

func joinURL(baseURL, path string) string {
	trimmedBase := strings.TrimRight(baseURL, "/")
	if strings.HasPrefix(path, "/") {
		return trimmedBase + path
	}
	return trimmedBase + "/" + path
}

func registerStaticDir(router *gin.Engine, routePrefix, staticDir, subdir string) {
	abs := absIfExists(staticDir)
	if abs == "" {
		return
	}
	assetsDir := filepath.Join(abs, subdir)
	if _, err := os.Stat(assetsDir); err == nil {
		router.Static(routePrefix, assetsDir)
	}
}

func registerStaticFile(router *gin.Engine, routePath, staticDir, filename string) {
	abs := absIfExists(staticDir)
	if abs == "" {
		return
	}
	filePath := filepath.Join(abs, filename)
	if _, err := os.Stat(filePath); err == nil {
		router.StaticFile(routePath, filePath)
	}
}

func serveIndexOr404(c *gin.Context, rootDir, fallbackMessage string) {
	if rootDir == "" {
		httpapi.Error(c, http.StatusNotFound, fallbackMessage)
		return
	}
	if _, err := fs.Stat(os.DirFS(rootDir), "index.html"); err == nil {
		c.File(filepath.Join(rootDir, "index.html"))
		return
	}
	httpapi.Error(c, http.StatusNotFound, fallbackMessage)
}

func serveSiteIndexOr404(c *gin.Context, rootDir, requestPath, fallbackMessage string) {
	if rootDir == "" {
		httpapi.Error(c, http.StatusNotFound, fallbackMessage)
		return
	}
	cleanPath := strings.TrimPrefix(filepath.Clean("/"+requestPath), "/")
	if cleanPath != "." && cleanPath != "" {
		routeIndex := filepath.Join(cleanPath, "index.html")
		if _, err := fs.Stat(os.DirFS(rootDir), routeIndex); err == nil {
			c.File(filepath.Join(rootDir, routeIndex))
			return
		}
	}
	serveIndexOr404(c, rootDir, fallbackMessage)
}

func absIfExists(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	if _, err := os.Stat(abs); err != nil {
		return ""
	}
	return abs
}

func currentUserID(c *gin.Context) uint64 {
	value, _ := c.Get(httpapi.ContextAppUserID)
	userID, _ := value.(uint64)
	return userID
}

func parsePort(addr string) (int, error) {
	parts := strings.Split(addr, ":")
	if len(parts) == 0 {
		return 0, fmt.Errorf("invalid HTTP_ADDR")
	}
	portStr := parts[len(parts)-1]
	port, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil {
		return 0, fmt.Errorf("invalid HTTP_ADDR: %w", err)
	}
	return port, nil
}

func hostPart(addr string) string {
	if strings.HasPrefix(addr, ":") || addr == "" {
		return "0.0.0.0"
	}
	idx := strings.LastIndex(addr, ":")
	if idx <= 0 {
		return "0.0.0.0"
	}
	return addr[:idx]
}

type apiKeyRepo struct{ store *sqlstore.Store }

func (r apiKeyRepo) FindByHash(ctx context.Context, hash string) (*model.APIKey, error) {
	return r.store.FindAPIKeyByHash(ctx, hash)
}
func (r apiKeyRepo) FindAPIKeyByHash(ctx context.Context, hash string) (*model.APIKey, error) {
	return r.store.FindAPIKeyByHash(ctx, hash)
}
func (r apiKeyRepo) TouchLastUsedAt(ctx context.Context, id uint64, usedAt time.Time) error {
	return r.store.TouchAPIKeyLastUsedAt(ctx, id, usedAt)
}
func (r apiKeyRepo) FindCLISessionByTokenHash(ctx context.Context, tokenHash string) (*model.CLISession, error) {
	return r.store.FindCLISessionByTokenHash(ctx, tokenHash)
}
func (r apiKeyRepo) TouchCLISession(ctx context.Context, id uint64, usedAt time.Time) error {
	return r.store.TouchCLISession(ctx, id, usedAt)
}
func (r apiKeyRepo) TouchAPIKeyLastUsedAt(ctx context.Context, id uint64, usedAt time.Time) error {
	return r.store.TouchAPIKeyLastUsedAt(ctx, id, usedAt)
}
func (r apiKeyRepo) ConsumePaidByHash(ctx context.Context, hash string) (*model.APIKey, error) {
	key, err := r.store.ConsumePaidQuotaByHash(ctx, hash)
	if err != nil && strings.Contains(err.Error(), "paid quota exhausted") {
		return nil, licensesvc.ErrPaidQuotaExhausted
	}
	return key, err
}
func (r apiKeyRepo) GetHostedCreditAccountByUser(ctx context.Context, userID uint64) (*model.UserHostedCreditAccount, error) {
	return r.store.GetHostedCreditAccountByUser(ctx, userID)
}

type usageEventRepo struct{ store *sqlstore.Store }

func (r usageEventRepo) Create(ctx context.Context, event *model.UsageEvent) error {
	return r.store.CreateUsageEvent(ctx, event)
}
func (r usageEventRepo) FindByRequestID(ctx context.Context, requestID string) (*model.UsageEvent, error) {
	return r.store.FindUsageEventByRequestID(ctx, requestID)
}
