#!/usr/bin/env python3
"""Runs ON THE VPS. Creates the MiroFish service products in the LIVE Stripe catalog.
Idempotent (skips existing by name / lookup_key). Per @hermes's delegated pricing design:
- MiroFish Market Intelligence: $199/mo, $1,990/yr, $249 one-time baseline report
- MiroFish A/B Messaging Test: $399 one-time (grants mirofish service access via bundle members)
"""
import json, urllib.request, urllib.parse, urllib.error, sys

def envkey():
    for line in open("/opt/katsironni/.env"):
        if line.startswith("STRIPE_SECRET_KEY="):
            return line.split("=", 1)[1].strip()
    raise SystemExit("no key")

KEY = envkey()
API = "https://api.stripe.com/v1"

def call(method, path, data=None):
    body = urllib.parse.urlencode(data).encode() if data is not None else None
    req = urllib.request.Request(API + path, data=body, method=method,
        headers={"Authorization": "Bearer " + KEY, "Content-Type": "application/x-www-form-urlencoded"})
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            return r.getcode(), json.load(r)
    except urllib.error.HTTPError as e:
        try: return e.code, json.load(e)
        except Exception: return e.code, {"error": {"message": "unparseable"}}

def flat(d, prefix=""):
    out = []
    for k, v in d.items():
        key = f"{prefix}[{k}]" if prefix else k
        if isinstance(v, dict): out += flat(v, key)
        elif isinstance(v, list):
            for item in v: out.append((key + "[]", item))
        else: out.append((key, v))
    return out

CATALOG = [
    ("mirofish", "MiroFish Market Intelligence",
     "Multi-agent market simulation: a digital sandbox of buyer/seller personas predicts demand, prices, and objections before you launch. Monthly monitoring, annual, or a one-time baseline report.",
     19900, 199000, 24900),
    ("mirofish-abtest", "MiroFish A/B Messaging Test",
     "Two parallel market simulations comparing announcement variants, with a comparative report: which message wins, why, and the exact buyer language that closed.",
     None, None, 39900),
]

code, prods = call("GET", "/products?limit=100")
prod_by_name = {p.get("name"): p for p in prods.get("data", [])}
code, prices = call("GET", "/prices?limit=100")
price_by_lookup = {p.get("lookup_key"): p for p in prices.get("data", []) if p.get("lookup_key")}

out = {"products": [], "prices": []}
for key, name, desc, monthly, yearly, onetime in CATALOG:
    if name in prod_by_name:
        pid = prod_by_name[name]["id"]
        print(f"[exists] product {name} -> {pid}")
    else:
        c, obj = call("POST", "/products", flat({"name": name, "description": desc, "active": "true",
            "metadata": {"service": key, "catalog": "mirofish"}}))
        if c != 200:
            print(f"[FAIL] product {name}: {obj}"); sys.exit(2)
        pid = obj["id"]
        print(f"[created] product {name} -> {pid} (livemode={obj.get('livemode')})")
    out["products"].append({"key": key, "name": name, "id": pid})

    tiers = []
    if monthly: tiers.append(("month", monthly, True))
    if yearly: tiers.append(("year", yearly, True))
    if onetime: tiers.append(("onetime", onetime, False))
    for interval, amount, recurring in tiers:
        lookup = f"{key}-{interval}"
        if lookup in price_by_lookup:
            prid = price_by_lookup[lookup]["id"]
            print(f"[exists] price {lookup}")
        else:
            params = {"product": pid, "currency": "cad", "unit_amount": amount, "lookup_key": lookup,
                      "metadata": {"service": key, "interval": interval}}
            if recurring: params["recurring"] = {"interval": interval}
            c, obj = call("POST", "/prices", flat(params))
            if c != 200:
                print(f"[FAIL] price {lookup}: {obj}"); sys.exit(2)
            prid = obj["id"]
            print(f"[created] price {lookup} -> {prid} ({amount}c {'recurring' if recurring else 'one-time'})")
        out["prices"].append({"key": key, "interval": interval, "unit_amount": amount, "id": prid, "recurring": recurring})

with open("/tmp/mirofish-ids.json", "w") as f:
    json.dump(out, f, indent=1)
print("MIROFISH CATALOG DONE")
