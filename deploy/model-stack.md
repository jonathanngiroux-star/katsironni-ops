# Model Stack Configuration (Startup — NVIDIA NIM Free Tier)

Configured 2026-10-26. Adjustments from the requested tier map noted below.

## Current stack (as deployed):

| Tier | Model | Status | Used for |
|------|-------|--------|----------|
| **Tier 1 (default)** | `minimaxai/minimax-m3.1-flash` via NIM | ✅ Working | Standard chat agents (all 22 service pages), quick responses |
| **Tier 2** | `nvidia/nemotron-3-ultra-550b-a55b` via NIM | ✅ Working | mirofish-bot agent + MiroFish simulation engine |
| **Tier 3 (premium)** | `nvidia/nemotron-3-ultra-550b-a55b` via NIM | ✅ Working (same as Tier 2) | Premium agent profiles when deployed ($199+/mo services) |
| **Fallback** | `z-ai/glm-5.3-flash` via OpenRouter | ✅ Working | NIM outage/rate-limit safety net |

## Adjustment from the requested mapping:
- **nemotron-3-super-120b-a12b** (requested for Tier 2) returns HTTP 404 — not available
  on the current NVIDIA NIM free-tier account. Substituted **nemotron-3-ultra** (the only
  working Nemotron model, confirmed by direct API probe). When the account gains super
  access, swap with: `hermes -p mirofish-bot config set model.default nvidia/nemotron-3-super-120b-a12b`

## Profile-level model overrides:
- **Global default** (all standard agents): minimax-m3.1-flash — set in main config.yaml
- **mirofish-bot**: nemotron-3-ultra — set in profiles/mirofish-bot/config.yaml
- **test-agent**: minimax-m3.1-flash (explicitly set to Tier 1)
- **Premium profiles** (when deployed): set per-profile to nemotron-3-ultra
  `hermes -p <premium-bot> config set model.default nvidia/nemotron-3-ultra-550b-a55b`

## MiroFish engine LLM config:
- `LLM_MODEL_NAME=nvidia/nemotron-3-ultra-550b-a55b` in /opt/katsironni/mirofish/backend/.env

## Post-revenue plan (DO NOT IMPLEMENT YET):
When the first client payment arrives:
1. Top up OpenRouter credits (start with ~$50-100)
2. Switch primary to a paid OpenRouter model (candidates: z-ai/glm-5.3, anthropic/claude-sonnet-4.5,
   openai/gpt-5.2 — pick based on benchmark results)
3. NIM becomes the fallback instead of OpenRouter
4. Result: no rate limits, better SLA, professional-grade reliability
5. Also consider requesting nemotron-3-super access from NVIDIA NIM
