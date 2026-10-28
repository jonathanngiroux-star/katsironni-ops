# katsironni.com — Approved Pricing Sheet (Phase 1)

**Status: APPROVED by user — "Approve as proposed" (2026-10-24).**
All prices CAD. Stripe live mode. Annual = 10× monthly (2 months free), except WalletRisk ($89/yr as published).
Positioning: self-hosted repos stay free forever; paid product = managed/hosted service on katsironni.com.

| # | Product | Repo | Monthly (CAD) | Annual (CAD) | Paid tier includes |
|---|---------|------|--------------|-------------|---------------------|
| 1 | Proofspan Cloud | proofspan | $19 (1900c) | $190 (19000c) | Hosted harness: web access, trace storage + retention, CI-gate API tokens, LangSmith/HoneyHive migration support |
| 2 | Switchyard Cloud | switchyard | $29 (2900c) | $290 (29000c) | Managed flag control plane incl. BSL cloud extras: hosted flags, SDK endpoints, environments, LD/Unleash migration support |
| 3 | Warmline Watch | warmline | $15 (1500c) | $150 (15000c) | Hosted reputation harness: managed queue, bounce/complaint classification, DKIM rotation reminders, weekly report |
| 4 | Oneprobe Hosted | oneprobe | $12 (1200c) | $120 (12000c) | Hosted monitor: target scraping, alert rules, webhook routing, retention, query API |
| 5 | WalletRisk Pro | walletrisk | $9 (900c) | $89 (8900c) | Scheduled multi-chain re-scans, email alerts on new approvals, deep reports (matches repo README) |
| 6 | AirdropIntel Early Access | airdropintel | $10 (1000c) | $100 (10000c) | Time-sensitive alerts 24–48h before free Friday digest (matches README) |
| 7 | Katsironni Complete (bundle) | all six | $59 (5900c) | $590 (59000c) | Everything above, one subscription — ~37% off à la carte |

À la carte totals: $94/mo, $939/yr.

## Payments
- Stripe Checkout (cards) — live mode, CAD — created ONLY per this sheet.
- Crypto (tx-hash flow): same CAD prices, live-rate conversion estimate at checkout, paid to user's receive-only addresses; verified via public chain APIs (EVM eth_getTransactionByHash, mempool.space BTC, Solana RPC, Tronscan, Stellar Horizon) checking recipient + amount + confirmations, then subscription activates.
- Receive-only addresses (user-provided, no keys anywhere):
  - EVM (Ethereum, Linea, Base, BNB, Polygon, Monad, Optimism, Arbitrum, Arc): 0x85ee7E71f762d772599cbF1EC20E651B30657521
  - BTC: bc1qxe2zx5tv3hdreaej6s2x4p7han85uey828rrhg
  - Solana: 4afnnpah1KYUF5T6mCfkdSo9guWcLQGvoEDnpqNZzfTw
  - Tron: TCJMp9eMGDzSM8bDCPB7QDz3KxqiuvBrW9
  - Stellar: GDKGPEO7CC46IRV7EUUWMITA3VA4447OZTFLGXDVXEZPV7VB3F5M7VA4

## Notes
- WalletRisk $9/$89 and AirdropIntel $10 match prices already published in the repos' READMEs — keep consistent.
- warmline + oneprobe READMEs say "donations only, no paid tier, nothing gated on money" → prep README copy tweaks (free self-host forever; managed hosting at katsironni.com) as PRs so repos and SaaS marketing don't contradict.
- Stripe account expected: acct_1UTjkv1DZgaA6oBE "Katsironni", livemode, CAD (verify via list_available_accounts_or_orgs before creating anything).
- Backend will need a user-created RESTRICTED Stripe API key (checkout write scope) — user creates it in Stripe Dashboard; never via chat.

## Phase status
- Phase 0 ✓ Tools verified: gh CLI (jonathanngiroux-star), Hostinger MCP (3 tools), Stripe MCP (11 tools), Resend MCP (137 tools) — all OAuth'd on katsironni profile.
- Phase 1 ✓ Pricing approved (this sheet).
- Phase 2 pending: session restart → verify Stripe context → create 7 products + 14 live prices → DNS pre-flight (katsironni.com zone) → VPS recreate (VM 1500693, Debian 13, pre-authorized; reconfirm before firing).
