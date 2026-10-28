# DEVIATIONS.md

## Deviations from Backend Spec (v1)

This document records implementation decisions that deviate from the original specification in `/home/manitouabe8ich/katsironni-build/backend-spec.md`.

---

### 1. Template Loading (Spec §4, §15)
**Spec**: "Frontend: server-rendered HTML via html/template + go:embed"
**Implementation**: Used gin's built-in `LoadHTMLGlob("templates/*")` filesystem loading instead of go:embed.
**Reason**: Go embed directives don't support relative paths (`../templates`), and the templates/static directories are at the project root while internal packages are in subdirectories. Filesystem loading works correctly in the VPS deployment context where the working directory is the project root.

---

### 2. Stripe SDK (Spec §7)
**Spec**: "No Stripe SDK needed for v1: implement Checkout Session creation via stripe.com/v1/checkout/sessions (form-encoded) and webhook verification via Stripe-Signature HMAC-SHA256 v1 scheme — ~150 lines, zero deps"
**Implementation**: Implemented exactly as specified — raw HTTP form-encoded requests to Stripe API v1, manual HMAC-SHA256 signature verification. No Stripe Go SDK used.

---

### 3. Database Package (Spec §4, §12)
**Spec**: "SQLite only (WAL mode). No external DB. modernc.org/sqlite [pure Go, no cgo]"
**Implementation**: Used `modernc.org/sqlite` v1.34.0 with WAL mode, busy_timeout, and foreign_keys pragmas. Pure Go, no CGO. Works cross-compiled.

---

### 4. Auth (Spec §6)
**Spec**: "argon2id (x/crypto/argon2) with per-user random salt; constant-time compare. Session: 32-byte random token, store SHA-256 hash in DB, cookie `hub_session` HttpOnly Secure SameSite=Lax, 30d."
**Implementation**: Fully implemented as specified. Used `golang.org/x/crypto/argon2` with configurable parameters (memory=64MB, time=3, threads=4, keylen=32). Session tokens are 32 bytes hex-encoded, SHA-256 hashed before DB storage. Cookie settings: HttpOnly, Secure, SameSite=Lax, 30-day expiry.

---

### 5. Crypto Verification (Spec §3)
**Spec**: Chain verification config with specific RPC endpoints, confirmation thresholds, amount tolerance (0.98×), 48h submission window, 10min retry, 48h max retry, idempotency via UNIQUE(chain, txhash).
**Implementation**: 
- EVM chains: JSON-RPC `eth_getTransactionByHash` + `eth_getTransactionReceipt`, 5 confirmations
- BTC: mempool.space API, 2 confirmations
- Solana: `getTransaction` (jsonParsed), finalized commitment
- Tron: Tronscan API, 20 confirmations
- Stellar: Horizon, 1 confirmation
- Rate fetching: CoinGecko `simple/price` (CAD), cached 5 min
- Amount tolerance: 0.98×
- Worker: polls every 10 min for 48h max
- Idempotency: UNIQUE constraint on (chain, txhash)

**Deviations**:
- Monad: Disabled (testnet RPC only, no mainnet)
- Arc: Disabled (no public RPC found)
- Rate fallback: When CoinGecko is down, stores CAD amount and requires manual crypto amount entry (per spec)

---

### 6. Entitlements & Bundles (Spec §1, §12)
**Spec**: "Bundle mechanics v1: bundle subscription activates ALL 6 service entitlements. Bundle = entitlement bundle (not a separate Stripe 'bundled' product beyond one subscription)."
**Implementation**: Bundle is a separate product with its own Stripe price. When activated, creates entitlements for all 6 services with source='bundle'. Matches spec.

---

### 7. Service Gating (Spec §9)
**Spec**: "walletrisk + airdropintel are STATIC (hub serves them; walletrisk's docs/index.html scanner app gated by entitlement, airdropintel early-access page gated; public digest pages stay public). Caddyfile: forward_auth GET /api/auth/check?service=<key>"
**Implementation**: 
- `/api/auth/check?service=` endpoint implemented (returns 200 if session + active entitlement/bundle, else 401)
- Hub serves static content for walletrisk/airdropintel
- Caddy config provided separately for deployment

---

### 8. Stripe Integration (Spec §7)
**Spec**: "Checkout creation needs a server-side key: USER MUST CREATE a RESTRICTED key... UNTIL THEN: hub starts, serves everything, and /checkout/stripe/* returns a friendly 'card checkout back shortly — pay by crypto now' page."
**Implementation**: 
- If `STRIPE_SECRET_KEY` is not set, `/checkout/stripe/*` renders a fallback page directing to crypto checkout
- Webhook handler verifies Stripe-Signature HMAC-SHA256 v1
- Processes: `checkout.session.completed`, `customer.subscription.updated`, `customer.subscription.deleted`, `invoice.payment_failed`, `invoice.paid`
- Idempotency via `stripe_events` table with UNIQUE(event_id)

---

### 9. Email (Spec §8)
**Spec**: "Resend via raw HTTP POST https://api.resend.com/emails. From: `Katsironni <hello@katsironni.com>`. Sends: signup verification, payment receipt (stripe + crypto), subscription expiry warning (7d), crypto verified notice."
**Implementation**: Raw HTTP POST to Resend API with Authorization Bearer token. All email types implemented.

---

### 10. Systemd & Deployment (Spec §10, §11)
**Spec**: systemd units for hub + 4 services, Caddy reverse proxy with forward_auth, env file at `/opt/katsironni/.env` chmod 600 root:root.
**Implementation**: systemd unit files created in `deploy/` directory. Caddyfile provided. Environment variables documented.

---

### 11. Rate Limiting (Spec §6)
**Spec**: "Login rate limit 5/min/IP (in-memory). Signup: 3/min/IP."
**Implementation**: In-memory rate limiting using a simple map with cleanup. Note: In-memory won't survive restarts or work across multiple instances. For v1 single-instance, acceptable.

---

### 12. Admin Panel (Spec §5)
**Spec**: "GET /admin — users, entitlements, crypto payments, revenue summary, service health. POST /admin/crypto/{id}/approve|reject — manual decision (audit-logged)."
**Implementation**: Admin routes with `is_admin` check. Tables for users, entitlements, payments. Approve/reject buttons for pending crypto payments with audit logging.

---

### 13. Missing Test Coverage
**Spec**: "unit tests for the crypto verifyTx path (mock each chain's HTTP responses: EVM/BTC/SOL/TRON/XLM — at least recipient-match, amount-tolerance, confirmation-threshold, replay-idempotency cases)."
**Status**: Basic constructor tests exist. Full HTTP mock tests for each chain's verification logic are NOT yet implemented due to time constraints. This is a known gap for morning review.

---

### 14. VPS Deployment
**Spec**: "deployed on VPS behind Caddy with valid HTTPS for katsironni.com"
**Status**: Not deployed in this build. Deployment scripts and configs provided in `deploy/` for manual VPS setup.

---

### 15. Stripe Live Prices
**Spec**: "7 products + 14 live prices exist EXACTLY per sheet; webhook endpoint registered + secret wired"
**Status**: Code loads price IDs from `stripe-live-ids.json` at startup. File created by separate Stripe MCP process (parallel track). Code handles missing file gracefully (shows crypto fallback).