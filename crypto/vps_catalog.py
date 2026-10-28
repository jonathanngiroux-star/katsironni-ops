#!/usr/bin/env python3
"""Runs ON THE VPS. Creates the approved Stripe catalog (7 products, 14 CAD prices, LIVE)
using the STRIPE_SECRET_KEY from /opt/katsironni/.env (key never printed).
Also registers the webhook endpoint and writes STRIPE_WEBHOOK_SECRET straight to .env.
Idempotent: skips objects that already exist (by name / lookup_key).
Writes /opt/katsironni/stripe-ids.json. Prints a full summary (IDs only, no secrets).
"""
import json, urllib.request, urllib.parse, urllib.error, sys

def envkey():
    for line in open("/opt/katsironni/.env"):
        if line.startswith("STRIPE_SECRET_KEY="):
            return line.split("=", 1)[1].strip()
    raise SystemExit("STRIPE_SECRET_KEY not found in .env")

KEY = envkey()
API = "https://api.stripe.com/v1"
ENVF = "/opt/katsironni/.env"

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

CATALOG = [
    ("proofspan", "Proofspan Cloud", "Hosted agent-eval harness: web access, trace storage + retention, CI-gate API tokens, LangSmith/HoneyHive migration support.", 1900, 19000),
    ("switchyard", "Switchyard Cloud", "Managed feature-flag control plane incl. BSL cloud extras: hosted flags, SDK endpoints, environments, LaunchDarkly/Unleash migration support.", 2900, 29000),
    ("warmline", "Warmline Watch", "Hosted reputation harness: managed queue, bounce/complaint classification, DKIM rotation reminders, weekly reputation report.", 1500, 15000),
    ("oneprobe", "Oneprobe Hosted", "Hosted infra monitor: target scraping, alert rules, webhook routing, retention, query API.", 1200, 12000),
    ("walletrisk", "WalletRisk Pro", "Scheduled multi-chain re-scans, email alerts on new approvals, deep reports.", 900, 8900),
    ("airdropintel", "AirdropIntel Early Access", "Time-sensitive alerts 24-48h before the free Friday digest.", 1000, 10000),
    ("bundle", "Katsironni Complete", "All six Katsironni services, one subscription (~37% off).", 5900, 59000),
]

# ---- 1. existing state ----
code, prods = call("GET", "/products?limit=100&expand[]=data.default_price")
code, prices = call("GET", "/prices?limit=100")
prod_by_name = {p.get("name"): p for p in prods.get("data", [])}
price_by_lookup = {p.get("lookup_key"): p for p in prices.get("data", []) if p.get("lookup_key")}
print(f"existing: {len(prods.get('data', []))} products, {len(prices.get('data', []))} prices")

result = {"account": "acct_1UTjkv1DZgaA6oBE", "livemode": True, "currency": "cad", "products": [], "prices": []}

# ---- 2. products ----
for key, name, desc, monthly, annual in CATALOG:
    if name in prod_by_name:
        pid = prod_by_name[name]["id"]
        print(f"[exists] product {name} -> {pid}")
    else:
        code, obj = call("POST", "/products", flat({
            "name": name, "description": desc, "active": "true",
            "metadata": {"service": key, **({"bundle": "true"} if key == "bundle" else {})}}))
        if code != 200:
            print(f"[FAIL] product {name}: HTTP {code} {obj.get('error', {}).get('message', '')[:120]}")
            sys.exit(2)
        pid = obj["id"]
        print(f"[created] product {name} -> {pid} (livemode={obj.get('livemode')})")
    result["products"].append({"key": key, "name": name, "id": pid})

# ---- 3. prices ----
for (key, name, desc, monthly, annual), prod in zip(CATALOG, result["products"]):
    for interval, amount in (("month", monthly), ("year", annual)):
        lookup = f"{key}-{interval}"
        if lookup in price_by_lookup:
            pr = price_by_lookup[lookup]
            print(f"[exists] price {lookup} -> {pr['id']} ({pr.get('unit_amount')}c)")
            prid = pr["id"]
        else:
            code, obj = call("POST", "/prices", flat({
                "product": prod["id"], "currency": "cad", "unit_amount": amount,
                "recurring": {"interval": interval},
                "nickname": f"{name} {'monthly' if interval == 'month' else 'annual'}",
                "lookup_key": lookup,
                "metadata": {"service": key, "interval": interval,
                             "bundle": "true" if key == "bundle" else "false"}}))
            if code != 200:
                print(f"[FAIL] price {lookup}: HTTP {code} {obj.get('error', {}).get('message', '')[:120]}")
                sys.exit(2)
            prid = obj["id"]
            print(f"[created] price {lookup} -> {prid} ({amount}c/{interval}, livemode={obj.get('livemode')})")
        result["prices"].append({"key": key, "interval": interval, "unit_amount": amount,
                                  "currency": "cad", "id": prid, "product_id": prod["id"],
                                  "lookup_key": lookup})

with open("/opt/katsironni/stripe-ids.json", "w") as f:
    json.dump(result, f, indent=2)
os_chmod = __import__("os").chmod
os_chmod("/opt/katsironni/stripe-ids.json", 0o640)
print(f"wrote /opt/katsironni/stripe-ids.json ({len(result['products'])} products, {len(result['prices'])} prices)")

# ---- 4. webhook endpoint ----
code, whs = call("GET", "/webhook_endpoints?limit=100")
existing_wh = [w for w in whs.get("data", []) if w.get("url") == "https://katsironni.com/webhooks/stripe" and w.get("status") == "enabled"]
if existing_wh:
    print(f"[exists] webhook endpoint {existing_wh[0]['id']} (secret already issued — leaving .env untouched unless absent)")
    secret = existing_wh[0].get("secret")
    have = any(l.startswith("STRIPE_WEBHOOK_SECRET=") for l in open(ENVF))
    if secret and not have:
        with open(ENVF, "a") as f:
            f.write(f"STRIPE_WEBHOOK_SECRET={secret}\n")
        print("webhook secret written to .env (was missing)")
    elif not secret and not have:
        print("NOTE: existing endpoint's secret not returned by API; if .env lacks STRIPE_WEBHOOK_SECRET, rotate/recreate the endpoint in the dashboard")
else:
    code, wh = call("POST", "/webhook_endpoints", flat({
        "url": "https://katsironni.com/webhooks/stripe",
        "description": "katsironni hub — subscription lifecycle",
        "enabled_events": ["checkout.session.completed", "customer.subscription.updated",
                           "customer.subscription.deleted", "invoice.paid", "invoice.payment_failed"]}))
    if code == 200:
        secret = wh.get("secret")
        with open(ENVF) as f:
            lines = [l for l in f if not l.startswith("STRIPE_WEBHOOK_SECRET=")]
        if secret:
            lines.append(f"STRIPE_WEBHOOK_SECRET={secret}\n")
        with open(ENVF, "w") as f:
            f.writelines(lines)
        print(f"[created] webhook endpoint {wh['id']} — secret written to .env (value length {len(secret or '')})")
    else:
        print(f"[WARN] webhook create failed: HTTP {code} {wh.get('error', {}).get('message', '')[:150]} — retry after reconsent or via dashboard")

print("VPS CATALOG SCRIPT COMPLETE")
