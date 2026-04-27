package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
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
	"github.com/officecli/officecli/platform/internal/discordoauth"
	growthsvc "github.com/officecli/officecli/platform/internal/growth"
	"github.com/officecli/officecli/platform/internal/hostedllm"
	"github.com/officecli/officecli/platform/internal/httpapi"
	licensesvc "github.com/officecli/officecli/platform/internal/license"
	"github.com/officecli/officecli/platform/internal/model"
	"github.com/officecli/officecli/platform/internal/objectstore"
	"github.com/officecli/officecli/platform/internal/officesdk"
	"github.com/officecli/officecli/platform/internal/previewshare"
	publishsvc "github.com/officecli/officecli/platform/internal/publish"
	rewardsvc "github.com/officecli/officecli/platform/internal/reward"
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
	Me(ctx context.Context, raw string) (*model.User, error)
	Logout(ctx context.Context, raw string) error
}

type adminRouteService interface {
	ResolveSession(raw string) (string, error)
	LoginURL(ctx context.Context, returnTo string) (string, error)
	HandleGoogleCallback(ctx context.Context, code, state string) (*admin.AdminIdentity, string, string, error)
	Login(ctx context.Context, password string) (string, error)
	CurrentIdentity(ctx context.Context, rawCookie string) (*admin.AdminIdentity, error)
	Logout(ctx context.Context, rawCookie string) error
	Overview(ctx context.Context) (*model.OverviewStats, error)
	ListAPIKeys(ctx context.Context) ([]model.APIKey, error)
	GetAPIKeyPlaintext(ctx context.Context, id uint64, actor string) (string, error)
	CreateAPIKey(ctx context.Context, req admin.CreateAPIKeyRequest) (*admin.CreateAPIKeyResponse, *model.APIKey, error)
	UpdateAPIKey(ctx context.Context, id uint64, req admin.UpdateAPIKeyRequest) error
	ListFreeQuotas(ctx context.Context, fingerprint string, usageDate string) ([]admin.DailyFreeQuotaView, error)
	UpdateFreeQuota(ctx context.Context, id uint64, freeLimit int) error
	ListUsageEvents(ctx context.Context, filter sqlstore.UsageEventFilter) ([]model.UsageEvent, error)
	ListUsers(ctx context.Context) ([]model.User, error)
	UpdateUser(ctx context.Context, id uint64, req admin.UpdateUserRequest) error
	ListOrders(ctx context.Context) ([]model.Order, error)
	UpdateOrder(ctx context.Context, id uint64, req admin.UpdateOrderRequest) error
	ListBillingEvents(ctx context.Context) ([]model.BillingEvent, error)
	Growth(ctx context.Context) (*admin.GrowthSnapshot, error)
	QuotaSources(ctx context.Context, filter admin.QuotaSourcesFilter) (*admin.QuotaSources, error)
	HostedPricingRules(ctx context.Context) ([]model.HostedPricingRule, error)
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
	growthService := growthsvc.NewService(dbStore, dbStore, dbStore, dbStore)
	apiKeyCipher, err := apikey.NewCipher(cfg.APIKeyEncryptionKey)
	if err != nil {
		return nil, err
	}
	lic := licensesvc.NewService(
		apiKeyRepo{store: dbStore},
		freeQuotaRepo{store: dbStore},
		usageEventRepo{store: dbStore},
		redisLicenseAdapter{store: redisRepo},
		rewardService,
		growthService,
		cfg.APIKeyHashSalt,
		cfg.DefaultFreeLimit,
		cfg.UsageIdempotencyTTL,
		licensesvc.ProofConfig{
			Seed: cfg.LicenseProofSeed,
			TTL:  cfg.LicenseProofTTL,
		},
	)
	hostedLLMSvc := hostedllm.NewService(dbStore, hostedllm.Config{
		BaseURL:    cfg.HostedLLMBaseURL,
		APIKey:     cfg.HostedLLMAPIKey,
		TextModel:  cfg.HostedLLMTextModel,
		ImageModel: cfg.HostedLLMImageModel,
		Provider:   cfg.HostedLLMProvider,
		HashSalt:   cfg.APIKeyHashSalt,
		TimeoutSec: 60,
	})
	adminGoogleProvider := auth.NewGoogleOAuthProvider(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.AdminGoogleRedirectURL)
	adminSvc := admin.NewService(dbStore, redisRepo, cfg.AdminPassword, cfg.AdminSessionTTL, "cop_admin_session", admin.NewSecureCookieCodec(cfg.SessionSecret), cfg.APIKeyHashSalt, apiKeyCipher, adminGoogleProvider, cfg.AdminGoogleAllowlist, hostedLLMSvc)
	authSvc := auth.NewService(auth.NewGoogleOAuthProvider(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GoogleRedirectURL), dbStore, redisRepo, "cop_app_session", cfg.AppSessionTTL, auth.NewSecureCookieCodec(cfg.AppSessionSecret), growthService, cfg.AppGoogleAllowlist)
	billingSvc := billing.NewService(dbStore, billing.NewStripeGateway(cfg.StripeSecretKey, cfg.StripeWebhookSecret, cfg.StripeSuccessURL, cfg.StripeCancelURL), cfg.PricingPacks)
	appSvc := appuser.NewService(dbStore, billingSvc, cfg.APIKeyHashSalt, apiKeyCipher, growthService)
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

	port, err := parsePort(cfg.HTTPAddr)
	if err != nil {
		return nil, err
	}
	server := egin.DefaultContainer().Build(egin.WithHost(hostPart(cfg.HTTPAddr)), egin.WithPort(port))
	registerRoutesWithHosted(server, cfg, lic, adminSvc, authSvc, appSvc, billingSvc, hostedLLMSvc, publishService, discordOAuthSvc, previewShares, sdkHandler, sdkProvider)

	application := ego.New()
	application.Serve(server)
	return &Application{ego: application}, nil
}

func (a *Application) Run() error { return a.ego.Run() }

func registerRoutes(r *egin.Component, cfg Config, lic *licensesvc.Service, adminSvc *admin.Service, authSvc *auth.Service, appSvc *appuser.Service, billingSvc *billing.Service, discordSvc discordOAuthRouteService) {
	registerRoutesWithHosted(r, cfg, lic, adminSvc, authSvc, appSvc, billingSvc, nil, nil, discordSvc, nil, nil, nil)
}

func registerRoutesWithHosted(r *egin.Component, cfg Config, lic *licensesvc.Service, adminSvc *admin.Service, authSvc *auth.Service, appSvc *appuser.Service, billingSvc *billing.Service, hostedSvc *hostedllm.Service, publishService publishRouteService, discordSvc discordOAuthRouteService, previewShares *previewshare.Service, sdkHandler *officesdk.Handler, sdkProvider *officesdk.FileProvider) {
	r.Use(httpapi.RequestIDMiddleware())
	r.Use(httpapi.AccessLogMiddleware(time.Second))
	r.Use(gin.Recovery())
	r.GET("/healthz", func(c *gin.Context) { httpapi.JSON(c, http.StatusOK, gin.H{"status": "ok"}) })
	api := r.Group("/api")

	registerLicenseRoutesWithConfig(api, cfg, lic)
	registerPublishRoutes(api, cfg, publishService)
	registerAuthRoutes(api, cfg, authSvc)
	registerAdminRoutes(api, cfg, adminSvc)
	api.GET("/pricing", func(c *gin.Context) { httpapi.JSON(c, http.StatusOK, appuserExternalPricing(billingSvc.Pricing())) })
	registerAppRoutes(api, cfg, authSvc, appSvc, billingSvc, discordSvc)
	registerStripeRoutes(api, billingSvc)
	registerPreviewRoutes(r, cfg, authSvc, previewShares, sdkHandler, sdkProvider)
	registerStatic(r.Engine, cfg)
}

func appuserExternalPricing(packs []model.PricingPack) []model.PricingPack {
	result := make([]model.PricingPack, 0, len(packs))
	for _, pack := range packs {
		if pack.PackKind != string(model.PackKindExternalGeneration) {
			continue
		}
		result = append(result, pack)
	}
	return result
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
			case strings.Contains(errMsg, "missing api key"), strings.Contains(errMsg, "invalid api key"), strings.Contains(errMsg, "disabled"):
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

func registerLicenseRoutesWithConfig(api *gin.RouterGroup, cfg Config, lic *licensesvc.Service) {
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
	}, lic)
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
		resp, err := hostedSvc.Complete(c.Request.Context(), c.GetHeader("Authorization"), req)
		if err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, gin.H{"content": resp.Content, "credit_balance": resp.CreditBalance})
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
		resp, err := hostedSvc.Complete(c.Request.Context(), c.GetHeader("Authorization"), req)
		if err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, gin.H{"content": resp.Content, "credit_balance": resp.CreditBalance})
	})
	llmAPI.POST("/structured", func(c *gin.Context) {
		var req hostedllm.CompletionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		req.RequestID = c.GetHeader("X-Request-Id")
		req.Kind = "structured"
		resp, err := hostedSvc.Complete(c.Request.Context(), c.GetHeader("Authorization"), req)
		if err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, gin.H{"content": resp.Content, "credit_balance": resp.CreditBalance})
	})
	llmAPI.POST("/image", func(c *gin.Context) {
		var req hostedllm.ImageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		req.RequestID = c.GetHeader("X-Request-Id")
		resp, err := hostedSvc.GenerateImage(c.Request.Context(), c.GetHeader("Authorization"), req)
		if err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, gin.H{
			"data":           base64.StdEncoding.EncodeToString(resp.Data),
			"mime":           resp.MIME,
			"credit_balance": resp.CreditBalance,
		})
	})
}

func registerAuthRoutes(api *gin.RouterGroup, cfg Config, authSvc authRouteService) {
	api.GET("/auth/google/login", func(c *gin.Context) {
		returnTo := c.Query("return_to")
		inviteCode := c.Query("invite")
		url, err := authSvc.LoginURL(c.Request.Context(), returnTo, inviteCode)
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		c.Redirect(http.StatusFound, url)
	})
	api.GET("/auth/google/callback", func(c *gin.Context) {
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
	})
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
	api.GET("/admin/auth/google/login", func(c *gin.Context) {
		returnTo := c.DefaultQuery("return_to", "/admin")
		url, err := adminSvc.LoginURL(c.Request.Context(), returnTo)
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		c.Redirect(http.StatusFound, url)
	})
	api.GET("/admin/auth/google/callback", func(c *gin.Context) {
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
	})
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
	protected.GET("/api-keys", func(c *gin.Context) {
		data, err := adminSvc.ListAPIKeys(c.Request.Context())
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
	protected.GET("/free-quotas", func(c *gin.Context) {
		data, err := adminSvc.ListFreeQuotas(c.Request.Context(), c.Query("fingerprint"), c.Query("usage_date"))
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, data)
	})
	protected.PATCH("/free-quotas/:id", func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var req admin.UpdateFreeQuotaRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpapi.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		if err := adminSvc.UpdateFreeQuota(c.Request.Context(), id, req.FreeLimit); err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		httpapi.JSON(c, http.StatusOK, gin.H{"success": true})
	})
	protected.GET("/usage-events", func(c *gin.Context) {
		filter := sqlstore.UsageEventFilter{Mode: c.Query("mode"), Result: c.Query("result"), ReasonCode: c.Query("reason_code"), Fingerprint: c.Query("fingerprint_hash")}
		if raw := c.Query("api_key_id"); raw != "" {
			parsed, _ := strconv.ParseUint(raw, 10, 64)
			filter.APIKeyID = &parsed
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
	protected.GET("/users", func(c *gin.Context) {
		data, err := adminSvc.ListUsers(c.Request.Context())
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
	sdkHandler.ServePageForFile(c, share.FileID)
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
	return joinURL(cfg.PlatformBaseURL, "/api/auth/google/login?"+values.Encode())
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
	proxy := httputil.NewSingleHostReverseProxy(parsed)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		publicHost := forwardedRequestHost(req)
		publicProto := forwardedRequestProto(req)
		originalDirector(req)
		if publicHost != "" {
			req.Host = publicHost
			req.Header.Set("X-Forwarded-Host", publicHost)
		}
		if publicProto != "" {
			req.Header.Set("X-Forwarded-Proto", publicProto)
			req.Header.Set("X-Forwarded-Port", forwardedRequestPort(publicHost, publicProto))
		}
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		rewriteOfficeSDKProxyLocation(resp)
		return nil
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

func registerAppRoutes(api *gin.RouterGroup, cfg Config, authSvc *auth.Service, appSvc *appuser.Service, billingSvc *billing.Service, discordSvc discordOAuthRouteService) {
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
	protected.GET("/orders", func(c *gin.Context) {
		data, err := billingSvc.ListOrdersByUser(c.Request.Context(), currentUserID(c))
		if err != nil {
			httpapi.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		filtered := make([]model.Order, 0, len(data))
		for _, order := range data {
			if order.PackKind != model.PackKindExternalGeneration {
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
			httpapi.JSON(c, http.StatusOK, order)
		case errors.Is(err, billing.ErrCheckoutSessionIDRequired):
			httpapi.Error(c, http.StatusBadRequest, err.Error())
		case errors.Is(err, billing.ErrOrderNotFound), errors.Is(err, billing.ErrOrderForbidden):
			httpapi.Error(c, http.StatusNotFound, err.Error())
		default:
			httpapi.Error(c, http.StatusBadGateway, err.Error())
		}
	})
	protected.GET("/pricing", func(c *gin.Context) {
		httpapi.JSON(c, http.StatusOK, appuserExternalPricing(billingSvc.Pricing()))
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
			serveIndexOr404(c, siteDir, "site ui not built")
			return
		}
		switch {
		case strings.HasPrefix(path, "/admin"):
			serveIndexOr404(c, adminDir, "admin ui not built")
		case strings.HasPrefix(path, "/app"):
			serveIndexOr404(c, appDir, "app ui not built")
		default:
			serveIndexOr404(c, siteDir, "site ui not built")
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

type freeQuotaRepo struct{ store *sqlstore.Store }

func (r freeQuotaRepo) GetOrCreateByFingerprint(ctx context.Context, fingerprint string, usageDate string, defaultLimit int) (*model.DailyFreeQuota, bool, error) {
	return r.store.GetOrCreateDailyFreeQuota(ctx, fingerprint, usageDate, defaultLimit)
}
func (r freeQuotaRepo) GetByFingerprint(ctx context.Context, fingerprint string, usageDate string) (*model.DailyFreeQuota, error) {
	return r.store.GetDailyFreeQuota(ctx, fingerprint, usageDate)
}
func (r freeQuotaRepo) Consume(ctx context.Context, fingerprint string, usageDate string, defaultLimit int) (*model.DailyFreeQuota, error) {
	quota, err := r.store.ConsumeDailyFreeQuota(ctx, fingerprint, usageDate, defaultLimit)
	if err != nil && strings.Contains(err.Error(), "quota exhausted") {
		return nil, licensesvc.ErrQuotaExhausted
	}
	return quota, err
}

type usageEventRepo struct{ store *sqlstore.Store }

func (r usageEventRepo) Create(ctx context.Context, event *model.UsageEvent) error {
	return r.store.CreateUsageEvent(ctx, event)
}
func (r usageEventRepo) FindByRequestID(ctx context.Context, requestID string) (*model.UsageEvent, error) {
	return r.store.FindUsageEventByRequestID(ctx, requestID)
}
