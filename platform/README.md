# OfficeCLI Platform

`officecli-platform` is the standalone licensing, billing, and control-plane service for OfficeCLI. It lives entirely under `platform/`, builds independently, manages its own frontend assets, and does not modify the sibling CLI implementation.

## Stack and Layout

- Backend: Go, ego, GORM, PostgreSQL, Redis
- Frontend: React, TypeScript, Vite, Ant Design, TanStack Query
- Local dependencies: Docker Compose for PostgreSQL and Redis
- Static hosting: the Go service serves `web/site/dist`, `web/app/dist`, and `web/admin/dist`

Key directories:

- `cmd/platform/main.go`: service entrypoint
- `internal/app`: configuration, routing, application wiring
- `internal/license`: license check and consume flows
- `internal/admin`: admin APIs for overview, users, orders, billing events, API keys, free quotas, and usage events
- `internal/auth`: Google sign-in and user session handling
- `internal/appuser`: end-user application APIs
- `internal/billing`: Stripe checkout, webhook, and order handling
- `internal/model`: data models
- `internal/store/sqlstore`: GORM-backed repositories
- `internal/store/redis`: sessions, idempotency, and locks
- `migrations/`: schema migrations
- `seed/`: demo data
- `web/site/`: public marketing site
- `web/app/`: user console
- `web/admin/`: admin console
- `deploy/docker-compose.yml`: local PostgreSQL and Redis

## Domain and Routing Plan

- `officecli.io`
  - `/`, `/pricing`, `/download`, `/faq`, `/docs`, `/login`
  - Public marketing content only
- `platform.officecli.io`
  - `/app`, `/app/*`: user console
  - `/admin`, `/admin/*`: internal admin console
  - `/api/license/*`, `/api/auth/*`, `/api/app/*`, `/api/admin/*`
  - `/api/stripe/webhook`, `/healthz`

The platform root redirects to `/app`. Requests for `/app` or `/admin` on the marketing domain should redirect to `platform.officecli.io`.

## Local Startup

Start dependencies:

```bash
docker compose -f deploy/docker-compose.yml up -d
```

Prepare environment variables:

```bash
cp .env.example .env
export $(grep -v '^#' .env | xargs)
```

Run migrations:

```bash
go run ./cmd/platform db migrate
```

Build frontend assets:

```bash
make web-install
make web-build
```

Start the service:

```bash
make dev
```

Local endpoints:

- Site: `http://127.0.0.1:8080/`
- App: `http://127.0.0.1:8080/app`
- Admin: `http://127.0.0.1:8080/admin`

Recommended production base URLs:

- `SITE_BASE_URL=https://officecli.io`
- `PLATFORM_BASE_URL=https://platform.officecli.io`

## Important Configuration

- `APP_ENV`: `development`, `staging`, or `production`
- `HTTP_ADDR`: HTTP listen address
- `POSTGRES_DSN`: PostgreSQL DSN
- `REDIS_ADDR`: Redis address
- `DEFAULT_FREE_LIMIT`: default one-time anonymous free quota per machine fingerprint
- `ADMIN_PASSWORD`: admin password
- `SESSION_SECRET`: admin session signing secret
- `APP_SESSION_SECRET`: app session signing secret
- `API_KEY_HASH_SALT`: API key hashing salt
- `SITE_BASE_URL`: site base URL
- `PLATFORM_BASE_URL`: platform base URL
- `GOOGLE_REDIRECT_URL`: Google OAuth callback URL
- `APP_GOOGLE_ALLOWLIST`: comma-separated allowlist for app sign-in; set to `*` to allow any Google account
- `STRIPE_SECRET_KEY`: Stripe server-side secret key; production must use a live key
- `STRIPE_WEBHOOK_SECRET`: Stripe webhook signing secret
- `STRIPE_SUCCESS_URL`: Stripe success redirect
- `STRIPE_CANCEL_URL`: Stripe cancel redirect
- `EXTERNAL_UNIT_PRICE_CENTS`: base external price per document, in cents
- `EXTERNAL_500_PRICE_RATIO`: discounted price ratio applied to the 500-document external pack
- `HOSTED_LLM_BASE_URL`: hosted LLM upstream base URL
- `HOSTED_LLM_API_KEY`: hosted LLM upstream API key
- `HOSTED_LLM_TEXT_MODEL`: hosted text model
- `HOSTED_LLM_IMAGE_MODEL`: hosted image model
- `HOSTED_LLM_PROVIDER`: hosted upstream provider id
- `HOSTED_PRICING_RULES_JSON`: hosted credit pricing rules; supported `document_profile` values are `text` and `image`
- `AIGATEWAY_ADMIN_BASE_URL`: aigateway management base URL for creating per-user upstream API keys
- `AIGATEWAY_ADMIN_API_KEY`: aigateway management bearer token; keep this secret out of code and logs
- `AIGATEWAY_API_KEY_GROUP`: optional aigateway group assigned to generated per-user upstream API keys
- `AIGATEWAY_CREATE_API_KEY_PATH`: aigateway management path for creating API keys

In production, the service must fail fast if required secrets are missing, still set to example values, or Stripe is configured with test keys.

## Operational Notes

- The paid runtime currently uses a quota-pack model.
- `POST /api/license/check` only evaluates availability.
- Actual quota consumption happens after a successful document generation through `POST /api/license/consume`.
- Logs are emitted as structured JSON on stdout.
- The service propagates or generates `X-Request-Id` and includes `request_id` in standard API responses.
- `/healthz` is intentionally excluded from normal access logging.

## Demo Data

Load demo free-quota data:

```bash
psql "host=127.0.0.1 port=5432 user=officecli password=officecli dbname=officecli_platform sslmode=disable" -f seed/demo.sql
```

Create test API keys from the admin console at `http://127.0.0.1:8080/admin`.

## CLI Contract Examples

License check:

```bash
curl -X POST https://platform.officecli.io/api/license/check \
  -H 'Content-Type: application/json' \
  -d '{
    "fingerprint_hash":"machine-1",
    "action":"generate"
  }'
```

License consume:

```bash
curl -X POST https://platform.officecli.io/api/license/consume \
  -H 'Content-Type: application/json' \
  -d '{
    "fingerprint_hash":"machine-1",
    "request_id":"req-1",
    "usage_type":"generate",
    "access_mode":"paid",
    "api_key":"cop_live_xxx"
  }'
```
