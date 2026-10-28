# katsironni-hub

Katsironni SaaS backend — single Go binary + SQLite. Serves marketing site, product pages, checkout (Stripe + crypto), auth, webhooks, crypto tx verification, admin panel, and gates 6 hosted services via forward_auth.

## Architecture

```
┌─────────────┐     ┌──────────────────┐     ┌─────────────────┐
│   Caddy     │────▶│  katsironni-hub  │────▶│   SQLite (WAL)  │
│  (TLS,      │     │   :8080          │     │   hub.db        │
│  forward_   │     │                  │     └─────────────────┘
│  auth)      │     │  • Auth          │
└─────────────┘     │  • Stripe        │
                    │  • Crypto        │     ┌─────────────────┐
                    │  • Entitlements  │────▶│  Stripe API     │
                    │  • Admin         │     │  (restricted    │
                    │  • Forward Auth  │     │   key)          │
                    └──────────────────┘     └─────────────────┘
                                              ┌─────────────────┐
                                              │  Resend API     │
                                              │  (email)        │
                                              └─────────────────┘
                                              ┌─────────────────┐
                                              │  Chain RPCs     │
                                              │  (EVM, BTC,     │
                                              │   SOL, TRX, XLM)│
                                              └─────────────────┘
```

## Quick Start (Local Dev)

```bash
# From project root
cd /home/manitouabe8ich/katsironni-build/hub

# Install dependencies
go mod download

# Build
go build ./cmd/hub/

# Run (requires env vars)
HUB_DB=./hub.db \
PUBLIC_URL=http://localhost:8080 \
STRIPE_SECRET_KEY=sk_test_... \
STRIPE_WEBHOOK_SECRET=whsec_... \
RESEND_API_KEY=re_... \
ADMIN_EMAIL=admin@example.com \
./hub
```

Visit `http://localhost:8080`

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `HUB_DB` | Yes | SQLite path (e.g., `/opt/katsironni/data/hub.db`) |
| `PUBLIC_URL` | Yes | Public URL (e.g., `https://katsironni.com`) |
| `STRIPE_SECRET_KEY` | No* | Restricted Stripe key (checkout write, customers write, subs read, invoices read) |
| `STRIPE_WEBHOOK_SECRET` | Yes | Stripe webhook signing secret |
| `RESEND_API_KEY` | Yes | Resend API key |
| `ADMIN_EMAIL` | No | Email of first admin (bootstrapped on first boot) |
| `CHAIN_CONFIG` | No | Path to chains.json (default: `/opt/katsironni/chains.json`) |
| `PORT` | No | HTTP port (default: 8080) |

*If `STRIPE_SECRET_KEY` is not set, card checkout shows a fallback page directing to crypto payment.

## Configuration Files

### chains.json
Chain verification config with RPC endpoints, addresses, confirmation thresholds. Located at `/opt/katsironni/chains.json` or set via `CHAIN_CONFIG`.

### stripe-live-ids.json
Stripe price IDs mapping (created via Stripe MCP). Loaded at startup from same directory as chains.json. If missing, crypto fallback is used.

```json
{
  "proofspan": {"month": "price_...", "year": "price_..."},
  "switchyard": {"month": "price_...", "year": "price_..."},
  ...
}
```

## Build & Test

```bash
# Build
go build ./cmd/hub/

# Vet
go vet ./...

# Test (includes crypto verifier unit tests)
go test ./...

# Run specific package tests
go test -v ./internal/crypto/
go test -v ./internal/auth/
```

## Project Structure

```
hub/
├── cmd/hub/main.go          # Entry point
├── internal/
│   ├── assets/              # Template loading
│   ├── auth/                # Argon2id auth, sessions
│   ├── config/              # Config, chains, pricing
│   ├── crypto/              # Chain verification (EVM, BTC, SOL, TRX, XLM)
│   ├── db/                  # SQLite + migrations
│   ├── email/               # Resend client
│   ├── entitlement/         # Service entitlements
│   ├── http/                # Gin handlers, routes
│   ├── stripe/              # Stripe checkout + webhooks
│   └── assets/              # Template loading
├── templates/               # HTML templates (gin)
├── static/css/style.css     # Stylesheet
├── config/chains.json       # Chain config
├── go.mod / go.sum
├── DEVIATIONS.md            # Spec deviations
└── README.md                # This file
```

## Deploy to VPS (Production)

### 1. Provision VPS
- Debian 13 (bookworm)
- Create user `katsironni` with sudo
- Install Go 1.27, Caddy

### 2. Deploy Binary & Config
```bash
# On VPS as root
mkdir -p /opt/katsironni/{data,services}
chown -R katsironni:katsironni /opt/katsironni

# Copy files
scp hub katsironni@VPS:/opt/katsironni/hub
scp -r templates static config katsironni@VPS:/opt/katsironni/
scp chains.json katsironni@VPS:/opt/katsironni/chains.json
scp stripe-live-ids.json katsironni@VPS:/opt/katsironni/stripe-live-ids.json
```

### 3. Create .env
```bash
# /opt/katsironni/.env (chmod 600, root:root)
HUB_DB=/opt/katsironni/data/hub.db
STRIPE_WEBHOOK_SECRET=whsec_...
STRIPE_SECRET_KEY=sk_live_...  # Restricted key
RESEND_API_KEY=re_...
ADMIN_EMAIL=admin@katsironni.com
PUBLIC_URL=https://katsironni.com
CHAIN_CONFIG=/opt/katsironni/chains.json
```

### 4. Install Systemd Units
```bash
# Copy unit files
scp deploy/*.service root@VPS:/etc/systemd/system/
scp deploy/Caddyfile root@VPS:/etc/caddy/Caddyfile

# Enable & start
systemctl daemon-reload
systemctl enable --now katsironni-hub
systemctl enable --now katsironni-proofspan
systemctl enable --now katsironni-switchyard
systemctl enable --now katsironni-warmline
systemctl enable --now katsironni-oneprobe
systemctl enable --now caddy
```

### 5. DNS & Caddy
- A @ → VPS IP
- AAAA @ → VPS IPv6
- Wildcard A *.katsironni.com → VPS IP
- Caddy auto-provisions TLS via Let's Encrypt

### 6. Verify
```bash
curl -I https://katsironni.com/healthz
# Should return 200 OK

# Test signup flow
# Test crypto checkout
# Test Stripe webhook (use Stripe CLI for local testing)
```

## Stripe Setup (Morning Task)

1. Verify Stripe account: `acct_1UTjkv1DZgaA6oBE` (Katsironni, livemode, CAD)
2. Create 7 products + 14 live prices per `pricing-approved.md`
3. Create RESTRICTED API key (Checkout Sessions: write, Customers: write, Subscriptions: read, Invoices: read)
4. Put key in `/opt/katsironni/.env` → `STRIPE_SECRET_KEY`
5. Register webhook endpoint: `https://katsironni.com/webhooks/stripe` with events:
   - `checkout.session.completed`
   - `customer.subscription.updated`
   - `customer.subscription.deleted`
   - `invoice.payment_failed`
   - `invoice.paid`
6. Copy webhook signing secret to `.env` → `STRIPE_WEBHOOK_SECRET`

## Resend Setup (Morning Task)

1. Create domain `katsironni.com` in Resend
2. Verify DNS (SPF, DKIM, DMARC via Hostinger)
3. Create API key (restricted to `katsironni.com` if possible)
4. Put in `.env` → `RESEND_API_KEY`

## Known Gaps (See DEVIATIONS.md)

- Crypto verifier HTTP mock tests not fully implemented
- VPS not yet provisioned
- Stripe live prices not yet created (parallel track)
- In-memory rate limiting (single-instance OK for v1)
- Monad/Arc chains disabled (no public RPC)

## License

MIT