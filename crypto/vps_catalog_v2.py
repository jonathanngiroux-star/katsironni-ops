#!/usr/bin/env python3
"""Runs ON THE VPS. Creates the APPROVED V2 Stripe catalog (23 products, 68 prices, LIVE)
per ~/katsironni-build/pricing-approved-v2.md. Uses STRIPE_SECRET_KEY from /opt/katsironni/.env
(key never printed). Idempotent: existing products (by name) and prices (by lookup_key) are
skipped. Writes /opt/katsironni/stripe-ids-v2.json. Prints a full summary (IDs only, no secrets).
"""
import json, urllib.request, urllib.parse, urllib.error, sys

def envkey():
    for line in open("/opt/katsironni/.env"):
        if line.startswith("STRIPE_SECRET_KEY="):
            return line.split("=", 1)[1].strip()
    raise SystemExit("STRIPE_SECRET_KEY not found in .env")

KEY = envkey()
API = "https://api.stripe.com/v1"
OUT = "/opt/katsironni/stripe-ids-v2.json"

def call(method, path, data=None):
    body = urllib.parse.urlencode(data).encode() if data is not None else None
    req = urllib.request.Request(API + path, data=body, method=method,
        headers={"Authorization": "Bearer " + KEY,
                 "Content-Type": "application/x-www-form-urlencoded"})
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            return r.getcode(), json.load(r)
    except urllib.error.HTTPError as e:
        try:
            return e.code, json.load(e)
        except Exception:
            return e.code, {"error": {"message": "unparseable", "code": f"HTTP{e.code}"}}

def flat(d, prefix=""):
    out = []
    for k, v in d.items():
        key = f"{prefix}[{k}]" if prefix else k
        if isinstance(v, dict):
            out += flat(v, key)
        elif isinstance(v, list):
            for item in v:
                out.append((key + "[]", item))
        else:
            out.append((key, v))
    return out

# (key, name, description, monthly_c, yearly_c, onetime_c)
CATALOG = [
    ("agentops", "Agent Ops Retainer", "Multi-agent orchestration hosting: failure recovery, version updates, 24/7 watching. Delivered as MCP server + managed agents.", 9900, 99000, 29700),
    ("contractaudit", "Smart Contract Continuous Audit", "Continuous smart-contract coverage: new deployments, upgrades, dependency changes — monthly, not one-shot. MCP-native.", 25000, 250000, 75000),
    ("walletguard", "Wallet Security Monitoring", "Phishing detection, address-risk continuous scanning, alert pipeline. Extension of WalletRisk. MCP-native.", 4900, 49000, 14700),
    ("nodeops", "Infra Node Monitoring", "Node health, RPC endpoint management, alert aggregation, DB optimization retainer. Extension of Oneprobe. MCP-native.", 10000, 100000, 30000),
    ("complianceflow", "Compliance Automation Retainer", "KYC/AML documentation cycles, MiCA/AMF regulatory-change tracking, weekly digests. Bilingual FR/EN.", 20000, 200000, 60000),
    ("taxfile", "Tax Filing Automation", "Periodic crypto-gains filing obligations, automated and monitored.", 9900, 99000, 29700),
    ("mcpretainer", "Custom MCP Server Retainer", "Build + maintain MCP servers on the Hermes stack, retainer-based.", 15000, 150000, 45000),
    ("agentbench", "Agent Evaluation Benchmark", "Continuous benchmark runs for AI agents. Extension of Proofspan. MCP-native.", 5900, 59000, 17700),
    ("tokenomics", "Tokenomics Modeling", "Recurring tokenomics model updates as parameters evolve.", 20000, 200000, 60000),
    ("whitepaper", "Whitepaper Drafting Retainer", "Living whitepaper updates (v2, v3) post-launch. Delivered via dedicated agent.", 30000, 300000, 90000),
    ("techdocs", "Technical Documentation Subscription", "Docs-as-a-service with bilingual FR/EN delivery.", 15000, 150000, 45000),
    ("regresearch", "Regulatory Research Subscription", "Jurisdiction watchlists, weekly summaries with citations.", 20000, 200000, 60000),
    ("datasetcur", "Dataset Curation Retainer", "Training/eval dataset refresh cycles.", 15000, 150000, 45000),
    ("finetune", "Fine-tuning Subscription", "Scheduled fine-tune runs + regression evals.", 25000, 250000, 75000),
    ("daogov", "DAO Governance Tooling", "Proposal tracking, voting analytics, delegate dashboards.", 15000, 150000, 45000),
    ("tokenlaunch", "Token Launch Support Retainer", "Pre-launch, TGE, and post-launch operational coverage.", 35000, 350000, 105000),
    ("meshops", "WireGuard Mesh Network Management", "Managed WireGuard mesh for distributed teams. MCP-native.", 9900, 99000, 29700),
    ("keymgmt", "Key Management Consulting", "Monthly key-ceremony and hardware-wallet security reviews.", 19900, 199000, 59700),
    ("emailops", "Email Delivery Optimization", "Deliverability + reputation retainer. Extension of Warmline. MCP-native.", 9900, 99000, 29700),
    ("flagstrategy", "Feature Flag Strategy", "Ongoing rollout-strategy reviews. Extension of Switchyard. MCP-native.", 9900, 99000, 29700),
    ("airdroppremium", "Airdrop Intel Premium", "Alerts + scoring API tier. Extension of AirdropIntel. MCP-native.", 2900, 29000, 8700),
    ("vpsops", "VPS Hosting Retainer", "Managed Caddy/Debian operations retainer.", 4900, 49000, 14700),
    ("certaintystack", "Certainty Stack", "Bundle: Wallet Security Monitoring + Infra Node Monitoring + Compliance Automation. The three highest-demand certainty services.", 19900, 199000, None),
]

# ---- 1. existing state ----
code, prods = call("GET", "/products?limit=100")
code, prices = call("GET", "/prices?limit=100")
prod_by_name = {p.get("name"): p for p in prods.get("data", [])}
price_by_lookup = {p.get("lookup_key"): p for p in prices.get("data", []) if p.get("lookup_key")}
print(f"existing: {len(prods.get('data', []))} products, {len(prices.get('data', []))} prices (incl. original catalog + archived probe)")

result = {"account": "acct_1UTjkv1DZgaA6oBE", "livemode": True, "currency": "cad", "products": [], "prices": []}

# ---- 2. products ----
for key, name, desc, monthly, yearly, onetime in CATALOG:
    if name in prod_by_name:
        pid = prod_by_name[name]["id"]
        print(f"[exists] product {name} -> {pid}")
    else:
        code, obj = call("POST", "/products", flat({
            "name": name, "description": desc, "active": "true",
            "metadata": {"service": key, "bundle": "true" if key == "certaintystack" else "false", "catalog": "v2"}}))
        if code != 200:
            print(f"[FAIL] product {name}: HTTP {code} {obj.get('error', {}).get('message', '')[:140]}")
            sys.exit(2)
        pid = obj["id"]
        print(f"[created] product {name} -> {pid} (livemode={obj.get('livemode')})")
    result["products"].append({"key": key, "name": name, "id": pid})

# ---- 3. prices: monthly + yearly (+ one-time where defined) ----
for (key, name, desc, monthly, yearly, onetime), prod in zip(CATALOG, result["products"]):
    tiers = [("month", monthly, True), ("year", yearly, True)]
    if onetime is not None:
        tiers.append(("onetime", onetime, False))
    for interval, amount, recurring in tiers:
        lookup = f"{key}-{interval}"
        if lookup in price_by_lookup:
            pr = price_by_lookup[lookup]
            print(f"[exists] price {lookup} -> {pr['id']} ({pr.get('unit_amount')}c)")
            prid = pr["id"]
        else:
            params = {
                "product": prod["id"], "currency": "cad", "unit_amount": amount,
                "lookup_key": lookup,
                "nickname": f"{name} {interval}",
                "metadata": {"service": key, "interval": interval,
                             "bundle": "true" if key == "certaintystack" else "false",
                             "one_time": "false" if recurring else "true"},
            }
            if recurring:
                params["recurring"] = {"interval": interval if interval != "onetime" else "month"}
            code, obj = call("POST", "/prices", flat(params))
            if code != 200:
                print(f"[FAIL] price {lookup}: HTTP {code} {obj.get('error', {}).get('message', '')[:140]}")
                sys.exit(2)
            prid = obj["id"]
            print(f"[created] price {lookup} -> {prid} ({amount}c {'recurring ' + interval if recurring else 'one-time'}, livemode={obj.get('livemode')})")
        result["prices"].append({"key": key, "interval": interval, "unit_amount": amount,
                                  "currency": "cad", "id": prid, "product_id": prod["id"],
                                  "lookup_key": lookup, "recurring": recurring})

import os
with open(OUT, "w") as f:
    json.dump(result, f, indent=2)
os.chmod(OUT, 0o640)
print(f"\nwrote {OUT}: {len(result['products'])} products, {len(result['prices'])} prices")

# ---- 4. verify: read back first product + first price ----
code, v1 = call("GET", f"/products/{result['products'][0]['id']}")
code, v2 = call("GET", f"/prices/{result['prices'][0]['id']}")
print(f"verify: '{v1.get('name')}' active={v1.get('active')} | {v2.get('id')} {v2.get('currency')} {v2.get('unit_amount')}c recurring={v2.get('recurring', {}).get('interval', 'one-time')}")
print("V2 CATALOG COMPLETE")
