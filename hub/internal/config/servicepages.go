package config

// ServicePage content for {service}.katsironni.com pages — copy sourced from the
// V2 market report (~/Documents/MiroFish/LABORX_TOP_25_REPORT_V2.md) plus the
// per-service quick actions for the chat widget.

type ServicePage struct {
	Icon         string   // emoji for the card
	Tagline      string   // one-liner for catalog cards
	Explain      string   // the specific "what this does" paragraph
	QuickActions []string // chat quick-action buttons
}

var ServicePages = map[string]ServicePage{
	"agentops": {
		Icon:         "🤖",
		Tagline:      "Someone is watching your agents, 24/7",
		Explain:      "Multi-agent orchestration hosting with failure recovery, version updates, and the payroll-grade reliability layer nobody else offers: when an agent fails, it is caught, recovered, and post-mortemed publicly. You get continuous oversight of your agent fleet — someone is watching, always.",
		QuickActions: []string{"Show agent fleet status", "Set up failure alerts", "Review recent incidents"},
	},
	"contractaudit": {
		Icon:         "🔍",
		Tagline:      "Every deployment, upgrade, and dependency change — audited continuously",
		Explain:      "Continuous smart-contract coverage: every new deployment, every proxy upgrade, every dependency change gets automatically analyzed for vulnerabilities. Not a one-shot audit you pay $20k for and forget — a standing watch over your contract surface, month after month.",
		QuickActions: []string{"Submit contract for audit", "Show coverage status", "Latest findings summary"},
	},
	"walletguard": {
		Icon:         "🛡️",
		Tagline:      "Continuous scanning for the drains you didn't see coming",
		Explain:      "Continuous scanning of your wallet addresses for phishing transactions, approval drainers, suspicious contract interactions, and blacklist appearances. Trading desks run this with contractual SLAs and adversarial test rights — you get the same pipeline from $49/mo.",
		QuickActions: []string{"Scan a wallet", "Watch a new address", "Recent alerts"},
	},
	"nodeops": {
		Icon:         "📡",
		Tagline:      "Node health, RPC endpoints, alerts — replaces a part-time employee",
		Explain:      "Node health monitoring, RPC endpoint management, alert aggregation, and DB optimization on retainer. Buyers compare it to the $150-300/mo part-time employee it replaces — you get structured maintenance with verification artifacts, not best-effort promises.",
		QuickActions: []string{"Add a node to watch", "RPC endpoint health", "Show alert history"},
	},
	"complianceflow": {
		Icon:         "📋",
		Tagline:      "KYC/AML cycles and MiCA/AMF tracking, weekly digests",
		Explain:      "KYC/AML documentation cycles, MiCA and AMF regulatory-change tracking, and weekly digests with citations. Bilingual FR/EN delivery — the exact service a French SME closed at $120/mo conditioned on verification artifacts and national-level coverage.",
		QuickActions: []string{"This week's compliance digest", "Track a new jurisdiction", "AMF watchlist status"},
	},
	"taxfile": {
		Icon:         "🧾",
		Tagline:      "Crypto-gains filing obligations, handled on schedule",
		Explain:      "Periodic crypto-gains filing obligations, automated and monitored. The rigid-obligation category that survives subscription fatigue: taxes don't care about market sentiment, and neither does the filing schedule.",
		QuickActions: []string{"Prepare current period", "Show filing calendar", "Export gains summary"},
	},
	"mcpretainer": {
		Icon:         "🔌",
		Tagline:      "We build and maintain your MCP servers",
		Explain:      "Custom MCP server construction and maintenance on the Hermes stack, retainer-based. Your AI tools get the integrations they need, and someone else keeps them working when upstream APIs change.",
		QuickActions: []string{"Scope a new MCP server", "Maintenance status", "Request a change"},
	},
	"agentbench": {
		Icon:         "📊",
		Tagline:      "Continuous benchmark runs, not one-shot evals",
		Explain:      "Continuous benchmark runs for AI agents — the proofspan engine applied as a standing service. Catch regressions as they happen with scheduled evaluation runs, CI-gated quality, and trend reports you can hand to stakeholders.",
		QuickActions: []string{"Run a benchmark now", "Show regression trends", "Schedule recurring runs"},
	},
	"tokenomics": {
		Icon:         "🪙",
		Tagline:      "Tokenomics models that evolve with your protocol",
		Explain:      "Recurring tokenomics model updates as your parameters evolve. Emissions, vesting, liquidity incentives — re-modeled on your cadence with the assumptions written down, so the model you show investors is never six months stale.",
		QuickActions: []string{"Update model parameters", "Current model summary", "Scenario analysis"},
	},
	"whitepaper": {
		Icon:         "📄",
		Tagline:      "A living whitepaper, not a launch-day artifact",
		Explain:      "Whitepaper updates as a living document — v2, v3, and beyond, post-launch. Your protocol ships weekly; the document describing it shouldn't freeze at TGE. Delivered via a dedicated agent that knows your repo, your model, and your changelog.",
		QuickActions: []string{"Draft next revision", "Summarize protocol changes", "Review current draft"},
	},
	"techdocs": {
		Icon:         "📚",
		Tagline:      "Docs-as-a-service, bilingual FR/EN",
		Explain:      "Technical documentation as a subscription — user guides, API references, and architecture docs that track your actual codebase. Bilingual FR/EN delivery, the underpriced moat for francophone Web3 projects.",
		QuickActions: []string{"Document a feature", "Audit existing docs", "Translate a page"},
	},
	"regresearch": {
		Icon:         "⚖️",
		Tagline:      "Jurisdiction watchlists with citations, weekly",
		Explain:      "Regulatory research subscription: jurisdiction watchlists, weekly summaries with citations. Know what changed in the places you operate in — before your counsel bills you four hours to discover it.",
		QuickActions: []string{"This week's regulatory summary", "Add a jurisdiction", "Citation archive"},
	},
	"datasetcur": {
		Icon:         "🗂️",
		Tagline:      "Training and eval datasets, refreshed on schedule",
		Explain:      "Dataset curation on retainer — training and evaluation dataset refresh cycles for AI-forward teams. Your evals are only as good as the freshest data in them; this keeps the data fresh.",
		QuickActions: []string{"Refresh eval dataset", "Dataset quality report", "Add new sources"},
	},
	"finetune": {
		Icon:         "🎯",
		Tagline:      "Scheduled fine-tunes with regression evals",
		Explain:      "Fine-tuning as a subscription: scheduled runs with regression evals so each generation of your model provably improves. No more ad-hoc tuning sessions with no baseline to compare against.",
		QuickActions: []string{"Schedule a fine-tune", "Latest regression report", "Compare model versions"},
	},
	"daogov": {
		Icon:         "🏛️",
		Tagline:      "Proposal tracking and voting analytics for your DAO",
		Explain:      "DAO governance tooling: proposal tracking, voting analytics, and delegate dashboards. See participation trends, delegate reliability, and proposal outcomes without building the pipeline yourself.",
		QuickActions: []string{"Active proposals", "Delegate analytics", "Voting participation report"},
	},
	"tokenlaunch": {
		Icon:         "🚀",
		Tagline:      "Pre-launch to TGE to post-launch, operationally covered",
		Explain:      "Token launch support on retainer: pre-launch preparation, TGE execution, and post-launch operational coverage. The phase most teams under-plan is the month AFTER launch — this covers it.",
		QuickActions: []string{"Launch readiness review", "TGE day-of plan", "Post-launch monitoring"},
	},
	"meshops": {
		Icon:         "🕸️",
		Tagline:      "Managed WireGuard mesh for distributed teams",
		Explain:      "Managed WireGuard mesh network operations for distributed teams. The seven-device mesh is the demo; your team's fleet is the product — configured, monitored, and maintained.",
		QuickActions: []string{"Mesh topology status", "Add a peer", "Connectivity diagnostics"},
	},
	"keymgmt": {
		Icon:         "🗝️",
		Tagline:      "Monthly key-ceremony and hardware reviews",
		Explain:      "Key management consulting on a monthly cadence: key-ceremony reviews, hardware-wallet policy checks, and operational security audits. High-trust-threshold security work delivered with verification artifacts.",
		QuickActions: []string{"Schedule a ceremony review", "Policy checklist", "Hardware audit status"},
	},
	"emailops": {
		Icon:         "✉️",
		Tagline:      "Deliverability and reputation, on retainer",
		Explain:      "Email delivery optimization as an ongoing retainer — the warmline reputation harness applied to your sending domain, with queue classification, DKIM rotation reminders, and weekly reputation reports.",
		QuickActions: []string{"Domain reputation report", "Deliverability check", "Bounce analysis"},
	},
	"flagstrategy": {
		Icon:         "🚩",
		Tagline:      "Rollout strategy reviews, continuously",
		Explain:      "Feature flag strategy as a service: ongoing rollout-strategy reviews on your switchyard data. Which flags are stale, which rollouts stalled, which experiments concluded — reviewed by someone who does this daily.",
		QuickActions: []string{"Flag inventory review", "Stale flag cleanup plan", "Rollout risk assessment"},
	},
	"airdroppremium": {
		Icon:         "🪂",
		Tagline:      "Airdrop alerts + scoring API, premium tier",
		Explain:      "Airdrop Intel Premium: time-sensitive alerts 24-48h before the free digest, plus the scoring API tier. Snapshots, claim windows, and points-program launches — first, not last.",
		QuickActions: []string{"This week's early alerts", "Scoring API status", "Claim window calendar"},
	},
	"vpsops": {
		Icon:    "🖥️",
		Tagline: "Managed Caddy/Debian operations",
		Explain: "VPS hosting retainer: managed Caddy and Debian operations for the stack you already run. Updates, certificates, firewall, and the small breakages before they become big ones.",
		QuickActions: []string{"Server health summary", "Pending updates", "Certificate status"},
	},
	"mirofish": {
		Icon:    "🐠",
		Tagline: "Rehearse your market before you launch — thousands of simulated buyers tell you what wins",
		Explain: "MiroFish builds a digital sandbox of your market: buyer and competitor personas with independent personalities, memory, and behavior interact across simulated social platforms while you inject variables from a God's-eye view. It predicted the price bands, objections, and exact closing language in katsironni.com's own market reports. Buy a one-time baseline report, an A/B messaging test (two variants, comparative verdict), or ongoing monthly monitoring.",
		QuickActions: []string{"Design a simulation", "What can MiroFish predict?", "Show sample report", "Set up A/B messaging test"},
	},
}
