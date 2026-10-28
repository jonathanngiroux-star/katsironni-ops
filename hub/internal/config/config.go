package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type ChainConfig struct {
	Key           string `json:"key"`
	Name          string `json:"name"`
	Asset         string `json:"asset"`
	RPCURL        string `json:"rpc_url"`
	VerifyMethod  string `json:"verify_method"`
	Confirmations int    `json:"confirmations"`
	Address       string `json:"address"`
	Enabled       bool   `json:"enabled"`
	RateID        string `json:"rate_id"`
}

type ChainsConfig struct {
	Chains                []ChainConfig `json:"chains"`
	RateCacheTTLSeconds   int           `json:"rate_cache_ttl_seconds"`
	AmountTolerance       float64       `json:"amount_tolerance"`
	SubmissionWindowHours int           `json:"submission_window_hours"`
	RetryIntervalMinutes  int           `json:"retry_interval_minutes"`
	MaxRetryHours         int           `json:"max_retry_hours"`
}

type Config struct {
	HubDB               string
	StripeWebhookSecret string
	StripeSecretKey     string
	ResendAPIKey        string
	AdminEmail          string
	PublicURL           string
	ChainConfigPath     string
	GoogleClientID      string
	GoogleClientSecret  string
	XClientID           string
	XClientSecret       string
	GitHubClientID      string
	GitHubClientSecret  string
	Chains              *ChainsConfig
}

func Load() (*Config, error) {
	cfg := &Config{
		HubDB:               getEnv("HUB_DB", "/opt/katsironni/data/hub.db"),
		StripeWebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
		StripeSecretKey:     os.Getenv("STRIPE_SECRET_KEY"),
		ResendAPIKey:        os.Getenv("RESEND_API_KEY"),
		AdminEmail:          os.Getenv("ADMIN_EMAIL"),
		PublicURL:           getEnv("PUBLIC_URL", "https://katsironni.com"),
		ChainConfigPath:     getEnv("CHAIN_CONFIG", "/opt/katsironni/chains.json"),
		GoogleClientID:      os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret:  os.Getenv("GOOGLE_CLIENT_SECRET"),
		XClientID:           os.Getenv("X_CLIENT_ID"),
		XClientSecret:       os.Getenv("X_CLIENT_SECRET"),
		GitHubClientID:      os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret:  os.Getenv("GITHUB_CLIENT_SECRET"),
	}

	// Load chains config
	if cfg.ChainConfigPath != "" {
		data, err := os.ReadFile(cfg.ChainConfigPath)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, err
			}
			// Use defaults if file doesn't exist
			cfg.Chains = defaultChainsConfig()
		} else {
			if err := json.Unmarshal(data, &cfg.Chains); err != nil {
				return nil, err
			}
		}
	} else {
		cfg.Chains = defaultChainsConfig()
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func defaultChainsConfig() *ChainsConfig {
	return &ChainsConfig{
		Chains:                []ChainConfig{},
		RateCacheTTLSeconds:   300,
		AmountTolerance:       0.98,
		SubmissionWindowHours: 48,
		RetryIntervalMinutes:  10,
		MaxRetryHours:         48,
	}
}

func (c *ChainsConfig) GetChain(key string) *ChainConfig {
	for i := range c.Chains {
		if c.Chains[i].Key == key {
			return &c.Chains[i]
		}
	}
	return nil
}

func (c *ChainsConfig) EnabledChains() []ChainConfig {
	var enabled []ChainConfig
	for _, chain := range c.Chains {
		if chain.Enabled {
			enabled = append(enabled, chain)
		}
	}
	return enabled
}

// Product pricing configuration
type ProductPricing struct {
	Key                  string
	Name                 string
	Description          string
	MonthlyCents         int
	AnnualCents          int
	OnetimeCents         int
	Live                 bool // on pricing page + purchasable (true once the service is deployed)
	StripeMonthlyPriceID string
	StripeAnnualPriceID  string
	StripeOnetimePriceID string
}

var Products = map[string]ProductPricing{
	"proofspan":    {Key: "proofspan", Name: "Proofspan Cloud", MonthlyCents: 1900, AnnualCents: 19000, Live: true, Description: "Hosted agent-eval harness: web access, trace storage + retention, CI-gate API tokens, LangSmith/HoneyHive migration support."},
	"switchyard":   {Key: "switchyard", Name: "Switchyard Cloud", MonthlyCents: 2900, AnnualCents: 29000, Live: true, Description: "Managed feature-flag control plane incl. BSL cloud extras: hosted flags, SDK endpoints, environments, LaunchDarkly/Unleash migration support."},
	"warmline":     {Key: "warmline", Name: "Warmline Watch", MonthlyCents: 1500, AnnualCents: 15000, Live: true, Description: "Hosted reputation harness: managed queue, bounce/complaint classification, DKIM rotation reminders, weekly reputation report."},
	"oneprobe":     {Key: "oneprobe", Name: "Oneprobe Hosted", MonthlyCents: 1200, AnnualCents: 12000, Live: true, Description: "Hosted infra monitor: target scraping, alert rules, webhook routing, retention, query API."},
	"walletrisk":   {Key: "walletrisk", Name: "WalletRisk Pro", MonthlyCents: 900, AnnualCents: 8900, Live: true, Description: "Scheduled multi-chain re-scans, email alerts on new approvals, deep reports."},
	"airdropintel": {Key: "airdropintel", Name: "AirdropIntel Early Access", MonthlyCents: 1000, AnnualCents: 10000, Live: true, Description: "Time-sensitive alerts 24-48h before the free Friday digest."},
	"bundle":       {Key: "bundle", Name: "Katsironni Complete", MonthlyCents: 5900, AnnualCents: 59000, Live: true, Description: "All six Katsironni services, one subscription (~37% off)."},

	// V2 catalog (approved 2026-10-26, pricing-approved-v2.md) — Live flips to true as each service deploys.
	"agentops":       {Key: "agentops", Name: "Agent Ops Retainer", MonthlyCents: 9900, AnnualCents: 99000, OnetimeCents: 29700, Description: "Multi-agent orchestration hosting: failure recovery, version updates, 24/7 watching. Delivered as MCP server + managed agents."},
	"contractaudit":  {Key: "contractaudit", Name: "Smart Contract Continuous Audit", MonthlyCents: 25000, AnnualCents: 250000, OnetimeCents: 75000, Description: "Continuous smart-contract coverage: new deployments, upgrades, dependency changes — monthly, not one-shot. MCP-native."},
	"walletguard":    {Key: "walletguard", Name: "Wallet Security Monitoring", MonthlyCents: 4900, AnnualCents: 49000, OnetimeCents: 14700, Description: "Phishing detection, address-risk continuous scanning, alert pipeline. Extension of WalletRisk. MCP-native.", Live: true},
	"nodeops":        {Key: "nodeops", Name: "Infra Node Monitoring", MonthlyCents: 10000, AnnualCents: 100000, OnetimeCents: 30000, Description: "Node health, RPC endpoint management, alert aggregation, DB optimization retainer. Extension of Oneprobe. MCP-native.", Live: true},
	"complianceflow": {Key: "complianceflow", Name: "Compliance Automation Retainer", MonthlyCents: 20000, AnnualCents: 200000, OnetimeCents: 60000, Description: "KYC/AML documentation cycles, MiCA/AMF regulatory-change tracking, weekly digests. Bilingual FR/EN."},
	"taxfile":        {Key: "taxfile", Name: "Tax Filing Automation", MonthlyCents: 9900, AnnualCents: 99000, OnetimeCents: 29700, Description: "Periodic crypto-gains filing obligations, automated and monitored."},
	"mcpretainer":    {Key: "mcpretainer", Name: "Custom MCP Server Retainer", MonthlyCents: 15000, AnnualCents: 150000, OnetimeCents: 45000, Description: "Build + maintain MCP servers on the Hermes stack, retainer-based."},
	"agentbench":     {Key: "agentbench", Name: "Agent Evaluation Benchmark", MonthlyCents: 5900, AnnualCents: 59000, OnetimeCents: 17700, Description: "Continuous benchmark runs for AI agents. Extension of Proofspan. MCP-native.", Live: true},
	"tokenomics":     {Key: "tokenomics", Name: "Tokenomics Modeling", MonthlyCents: 20000, AnnualCents: 200000, OnetimeCents: 60000, Description: "Recurring tokenomics model updates as parameters evolve."},
	"whitepaper":     {Key: "whitepaper", Name: "Whitepaper Drafting Retainer", MonthlyCents: 30000, AnnualCents: 300000, OnetimeCents: 90000, Description: "Living whitepaper updates (v2, v3) post-launch. Delivered via dedicated agent."},
	"techdocs":       {Key: "techdocs", Name: "Technical Documentation Subscription", MonthlyCents: 15000, AnnualCents: 150000, OnetimeCents: 45000, Description: "Docs-as-a-service with bilingual FR/EN delivery", Live: true},
	"regresearch":    {Key: "regresearch", Name: "Regulatory Research Subscription", MonthlyCents: 20000, AnnualCents: 200000, OnetimeCents: 60000, Description: "Jurisdiction watchlists, weekly summaries with citations", Live: true},
	"datasetcur":     {Key: "datasetcur", Name: "Dataset Curation Retainer", MonthlyCents: 15000, AnnualCents: 150000, OnetimeCents: 45000, Description: "Training/eval dataset refresh cycles", Live: true},
	"finetune":       {Key: "finetune", Name: "Fine-tuning Subscription", MonthlyCents: 25000, AnnualCents: 250000, OnetimeCents: 75000, Description: "Scheduled fine-tune runs + regression evals", Live: true},
	"daogov":         {Key: "daogov", Name: "DAO Governance Tooling", MonthlyCents: 15000, AnnualCents: 150000, OnetimeCents: 45000, Description: "Proposal tracking, voting analytics, delegate dashboards", Live: true},
	"tokenlaunch":    {Key: "tokenlaunch", Name: "Token Launch Support Retainer", MonthlyCents: 35000, AnnualCents: 350000, OnetimeCents: 105000, Description: "Pre-launch, TGE, and post-launch operational coverage", Live: true},
	"meshops":        {Key: "meshops", Name: "WireGuard Mesh Network Management", MonthlyCents: 9900, AnnualCents: 99000, OnetimeCents: 29700, Description: "Managed WireGuard mesh for distributed teams. MCP-native", Live: true},
	"keymgmt":        {Key: "keymgmt", Name: "Key Management Consulting", MonthlyCents: 19900, AnnualCents: 199000, OnetimeCents: 59700, Description: "Monthly key-ceremony and hardware-wallet security reviews", Live: true},
	"emailops":       {Key: "emailops", Name: "Email Delivery Optimization", MonthlyCents: 9900, AnnualCents: 99000, OnetimeCents: 29700, Description: "Deliverability + reputation retainer. Extension of Warmline. MCP-native.", Live: true},
	"flagstrategy":   {Key: "flagstrategy", Name: "Feature Flag Strategy", MonthlyCents: 9900, AnnualCents: 99000, OnetimeCents: 29700, Description: "Ongoing rollout-strategy reviews. Extension of Switchyard. MCP-native.", Live: true},
	"airdroppremium": {Key: "airdroppremium", Name: "Airdrop Intel Premium", MonthlyCents: 2900, AnnualCents: 29000, OnetimeCents: 8700, Description: "Alerts + scoring API tier. Extension of AirdropIntel. MCP-native.", Live: true},
	"vpsops":         {Key: "vpsops", Name: "VPS Hosting Retainer", MonthlyCents: 4900, AnnualCents: 49000, OnetimeCents: 14700, Description: "Managed Caddy/Debian operations retainer."},
	"certaintystack": {Key: "certaintystack", Name: "Certainty Stack", MonthlyCents: 19900, AnnualCents: 199000, Description: "Bundle: Wallet Security Monitoring + Infra Node Monitoring + Compliance Automation. The three highest-demand certainty services."},

	// MiroFish (deployed 2026-10-26)
	"mirofish":       {Key: "mirofish", Name: "MiroFish Market Intelligence", MonthlyCents: 19900, AnnualCents: 199000, OnetimeCents: 24900, Live: true, Description: "Multi-agent market simulation: a digital sandbox of buyer/seller personas predicts demand, prices, and objections before you launch. Monthly monitoring, annual, or a one-time baseline report."},
	"mirofish-abtest": {Key: "mirofish-abtest", Name: "MiroFish A/B Messaging Test", OnetimeCents: 39900, Live: true, Description: "Two parallel market simulations comparing announcement variants, with a comparative report: which message wins, why, and the exact buyer language that closed."},
}

// BundleMembers maps each bundle product to the services it grants.
var BundleMembers = map[string][]string{
	"bundle":        {"proofspan", "switchyard", "warmline", "oneprobe", "walletrisk", "airdropintel"},
	"certaintystack": {"walletguard", "nodeops", "complianceflow"},
	// A/B test purchase grants access to the mirofish service chat (agent handles the runs).
	"mirofish-abtest": {"mirofish"},
}

func GetProduct(key string) (ProductPricing, bool) {
	p, ok := Products[key]
	return p, ok
}

var productOrder = []string{
	"proofspan", "switchyard", "warmline", "oneprobe", "walletrisk", "airdropintel", "bundle",
	"agentops", "contractaudit", "walletguard", "nodeops", "complianceflow", "taxfile",
	"mcpretainer", "agentbench", "tokenomics", "whitepaper", "techdocs", "regresearch",
	"datasetcur", "finetune", "daogov", "tokenlaunch", "meshops", "keymgmt",
	"emailops", "flagstrategy", "airdroppremium", "vpsops", "certaintystack",
	"mirofish", "mirofish-abtest",
}

func AllProducts() []ProductPricing {
	var result []ProductPricing
	for _, k := range productOrder {
		if p, ok := Products[k]; ok {
			result = append(result, p)
		}
	}
	return result
}

// LiveProducts returns only products whose backing service is deployed.
func LiveProducts() []ProductPricing {
	var result []ProductPricing
	for _, k := range productOrder {
		if p, ok := Products[k]; ok && p.Live {
			result = append(result, p)
		}
	}
	return result
}

// LoadStripePriceIDs loads Stripe price IDs from stripe-live-ids.json
func LoadStripePriceIDs(cfg *Config) error {
	// Look for stripe-live-ids.json in config directory
	path := filepath.Join(filepath.Dir(cfg.ChainConfigPath), "stripe-live-ids.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet - that's OK for now
			return nil
		}
		return err
	}

	var priceIDs map[string]map[string]string // product_key -> interval -> price_id
	if err := json.Unmarshal(data, &priceIDs); err != nil {
		return err
	}

	for productKey, intervals := range priceIDs {
		if p, ok := Products[productKey]; ok {
			p.StripeMonthlyPriceID = intervals["month"]
			p.StripeAnnualPriceID = intervals["year"]
			p.StripeOnetimePriceID = intervals["onetime"]
			Products[productKey] = p
		}
	}

	// Also load the V2 catalog manifest (stripe-ids-v2.json, richer format) if present.
	v2Path := filepath.Join(filepath.Dir(cfg.ChainConfigPath), "stripe-ids-v2.json")
	if data, err := os.ReadFile(v2Path); err == nil {
		var v2 struct {
			Prices []struct {
				Key      string `json:"key"`
				Interval string `json:"interval"`
				ID       string `json:"id"`
			} `json:"prices"`
		}
		if json.Unmarshal(data, &v2) == nil {
			for _, pr := range v2.Prices {
				if p, ok := Products[pr.Key]; ok {
					switch pr.Interval {
					case "month":
						p.StripeMonthlyPriceID = pr.ID
					case "year":
						p.StripeAnnualPriceID = pr.ID
					case "onetime":
						p.StripeOnetimePriceID = pr.ID
					}
					Products[pr.Key] = p
				}
			}
		}
	}

	return nil
}
