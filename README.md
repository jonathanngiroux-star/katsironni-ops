# katsironni-ops

Deployment and operations tooling for **katsironni.com** — the VPS provisioning scripts, the
service hub (Go), the Stripe price catalogue, and the agent skill definitions.

This repo was extracted from the former `katsironni-platform` monorepo so that each concern
lives in one place:

| Path | Contents |
|------|----------|
| `deploy/` | VPS provisioning, tier-1/tier-2 rollout scripts, Caddyfile, systemd units, agent profile docs |
| `deploy/systemd/` | systemd service definitions for the hub, mirofish, oneprobe, proofspan, switchyard, warmline |
| `deploy/mirofish-skill/SKILL.md` | MiroFish operator skill |
| `hub/` | Go service hub — auth (API keys, OAuth, SIWE), entitlements, Stripe checkout, chat WebSocket, crypto payments, templates |
| `crypto/` | Chain catalogue and Stripe product/price catalogue creation scripts |
| `stripe/` | Approved pricing sheets and the live Stripe product/price ID catalogue |
| `agents/` | Per-service agent skills |

## Secrets

No credentials are committed. Scripts read them from the deployment environment:

- `STRIPE_SECRET_KEY` — read from `/opt/katsironni/.env` by `crypto/vps_catalog*.py`
- `NVIDIA_API_KEY`, `OPENROUTER_API_KEY` — read from `/opt/katsironni/.hermes/.env` by the deploy scripts
- `STRIPE_WEBHOOK_SECRET` — written to `.env` by `crypto/vps_catalog.py` when the webhook endpoint is created

The files under `stripe/` contain **product and price IDs** (not keys). Those IDs are
non-secret identifiers, but treat the live catalogue as internal pricing configuration.

## Usage

```bash
# Provision a new VPS
./deploy/provision-vps.sh <host>

# Roll out tier 1 / tier 2 services
./deploy/deploy_tier1.sh
./deploy/deploy_tier2.sh

# Register the Stripe catalogue (requires STRIPE_SECRET_KEY in /opt/katsironni/.env)
python3 crypto/vps_catalog_v2.py
```

The Go hub builds with:

```bash
cd hub && go build ./...
```

## Related repos

- Service implementations: `mcp-*` repos on this account
- MiroFish service: `mirofish-service`
- MiroFish platform fork: `MiroFish`

## License

See `LICENSE`.
