#!/usr/bin/env python3
"""Create the approved katsironni.com Stripe catalog (7 products, 14 CAD prices) in LIVE mode.
Idempotent: lists existing products/prices first and only creates what's missing.
Writes /home/manitouabe8ich/katsironni-build/stripe-live-ids.json.
Approved sheet: ~/katsironni-build/pricing-approved.md (amounts in cents).
"""
import json, sys, re, time
sys.path.insert(0, "/home/manitouabe8ich/katsironni-build/mcp")
import importlib.util
spec = importlib.util.spec_from_file_location("mcpmod", "/home/manitouabe8ich/katsironni-build/mcp/mcp.py")
mcp = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mcp)

CTX = "acct_1UTjkv1DZgaA6oBE"
OUT = "/home/manitouabe8ich/katsironni-build/stripe-live-ids.json"

CATALOG = [
    ("proofspan", "Proofspan Cloud", "Hosted agent-eval harness: web access, trace storage + retention, CI-gate API tokens, LangSmith/HoneyHive migration support.", 1900, 19000),
    ("switchyard", "Switchyard Cloud", "Managed feature-flag control plane incl. BSL cloud extras: hosted flags, SDK endpoints, environments, LaunchDarkly/Unleash migration support.", 2900, 29000),
    ("warmline", "Warmline Watch", "Hosted reputation harness: managed queue, bounce/complaint classification, DKIM rotation reminders, weekly reputation report.", 1500, 15000),
    ("oneprobe", "Oneprobe Hosted", "Hosted infra monitor: target scraping, alert rules, webhook routing, retention, query API.", 1200, 12000),
    ("walletrisk", "WalletRisk Pro", "Scheduled multi-chain re-scans, email alerts on new approvals, deep reports.", 900, 8900),
    ("airdropintel", "AirdropIntel Early Access", "Time-sensitive alerts 24-48h before the free Friday digest.", 1000, 10000),
    ("bundle", "Katsironni Complete", "All six Katsironni services, one subscription (~37% off).", 5900, 59000),
]

tok, client, meta = mcp.load("stripe")
token = mcp.get_access_token("stripe", tok, client, meta)
BASE = mcp.SERVERS["stripe"]

def call(tool, args, retries=2):
    for attempt in range(retries + 1):
        token_now = mcp.get_access_token("stripe", *mcp.load("stripe"))
        out = mcp.tool_call(BASE, token_now, tool, args)
        if not out.get("isError"):
            return out["text"]
        if attempt < retries:
            print(f"  [retry {attempt+1}] {tool} error: {out['text'][:200]}", flush=True)
            time.sleep(2)
        else:
            raise RuntimeError(f"{tool} failed: {out['text'][:400]}")

def parse_obj(text):
    """Tolerantly extract the JSON object from a tool response."""
    try:
        return json.loads(text)
    except Exception:
        m = re.search(r"\{.*\}", text, re.S)
        if m:
            try:
                return json.loads(m.group(0))
            except Exception:
                pass
        idm = re.search(r'"id"\s*:\s*"([^"]+)"', text)
        if idm:
            return {"id": idm.group(1)}
    raise RuntimeError(f"unparseable response: {text[:300]}")

# ---- 1. Existing state ----
print("== listing existing products & prices ==", flush=True)
prods_text = call("stripe_api_read", {"stripe_api_operation_id": "GetProducts", "parameters": {"limit": 100}, "stripe_context": CTX, "livemode": True})
prices_text = call("stripe_api_read", {"stripe_api_operation_id": "GetPrices", "parameters": {"limit": 100}, "stripe_context": CTX, "livemode": True})
prods = parse_obj(prods_text).get("data", [])
prices = parse_obj(prices_text).get("data", [])
prod_by_name = {p.get("name"): p for p in prods}
price_by_lookup = {p.get("lookup_key"): p for p in prices if p.get("lookup_key")}
print(f"   existing: {len(prods)} products, {len(prices)} prices", flush=True)

result = {"account_id": CTX, "livemode": True, "currency": "cad", "products": [], "prices": []}

# ---- 2. Products ----
for key, name, desc, monthly, annual in CATALOG:
    if name in prod_by_name:
        pid = prod_by_name[name]["id"]
        print(f"[exists] product {name} -> {pid}", flush=True)
    else:
        txt = call("stripe_api_write", {
            "stripe_api_operation_id": "PostProducts",
            "parameters": {"name": name, "description": desc, "active": True,
                          "metadata": {"service": key, **({"bundle": "true"} if key == "bundle" else {})}},
            "stripe_context": CTX, "livemode": True})
        pid = parse_obj(txt)["id"]
        print(f"[created] product {name} -> {pid}", flush=True)
    result["products"].append({"key": key, "name": name, "id": pid})

# ---- 3. Prices ----
for (key, name, desc, monthly, annual), prod in zip(CATALOG, result["products"]):
    for interval, amount in (("month", monthly), ("year", annual)):
        lookup = f"{key}-{interval}"
        if lookup in price_by_lookup:
            pr = price_by_lookup[lookup]
            print(f"[exists] price {lookup} -> {pr['id']} ({pr.get('unit_amount')}c)", flush=True)
            prid = pr["id"]
        else:
            txt = call("stripe_api_write", {
                "stripe_api_operation_id": "PostPrices",
                "parameters": {
                    "product": prod["id"], "currency": "cad", "unit_amount": amount,
                    "recurring": {"interval": interval},
                    "nickname": f"{name} {'monthly' if interval == 'month' else 'annual'}",
                    "lookup_key": lookup,
                    "metadata": {"service": key, "interval": interval, "bundle": "true" if key == "bundle" else "false"},
                },
                "stripe_context": CTX, "livemode": True})
            obj = parse_obj(txt)
            prid = obj["id"]
            print(f"[created] price {lookup} -> {prid} ({amount}c/{interval})", flush=True)
        result["prices"].append({"key": key, "interval": interval, "unit_amount": amount,
                                  "currency": "cad", "id": prid, "product_id": prod["id"]})

# ---- 4. Persist ----
with open(OUT, "w") as f:
    json.dump(result, f, indent=2)
print(f"\nwrote {OUT}: {len(result['products'])} products, {len(result['prices'])} prices", flush=True)

# ---- 5. Verify: read back one product and one price ----
v1 = call("stripe_api_read", {"stripe_api_operation_id": "GetProductsId", "parameters": {"id": result["products"][0]["id"]}, "stripe_context": CTX, "livemode": True})
v2 = call("stripe_api_read", {"stripe_api_operation_id": "GetPricesPrice", "parameters": {"id": result["prices"][0]["id"]}, "stripe_context": CTX, "livemode": True})
p1, p2 = parse_obj(v1), parse_obj(v2)
print(f"verify: product '{p1.get('name')}' active={p1.get('active')} | price {p2.get('id')} {p2.get('currency')} {p2.get('unit_amount')}c recurring={p2.get('recurring',{}).get('interval')} livemode-object={p2.get('livemode')}", flush=True)
print("CATALOG CREATION COMPLETE", flush=True)
