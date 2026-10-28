#!/usr/bin/env python3
"""Generalized MCP driver for the katsironni overnight build.
Usage:
  mcp.py <server> tools
  mcp.py <server> call <tool> '<json-args>'
Servers: hostinger | stripe | resend  (OAuth tokens cached in the katsironni profile).
Never prints tokens. Streamable HTTP + SSE, same proven logic as the pre-flight driver.
"""
import json, time, sys, urllib.request, urllib.parse, urllib.error

PROFILE = "/home/manitouabe8ich/.hermes/profiles/katsironni"
SERVERS = {
    "hostinger": "https://mcp.hostinger.com",
    "stripe": "https://mcp.stripe.com",
    "resend": "https://mcp.resend.com/mcp",
}
UA = "Mozilla/5.0 (X11; Linux x86_64; rv:131.0) Gecko/20100101 Firefox/131.0"

def load(server):
    tok = json.load(open(f"{PROFILE}/mcp-tokens/{server}.json"))
    client = json.load(open(f"{PROFILE}/mcp-tokens/{server}.client.json"))
    try:
        meta = json.load(open(f"{PROFILE}/mcp-tokens/{server}.meta.json"))
    except FileNotFoundError:
        meta = {}
    return tok, client, meta

def get_access_token(server, tok, client, meta):
    if tok.get("expires_at", 0) - 120 > time.time() and tok.get("access_token"):
        return tok["access_token"]
    endpoint = meta["token_endpoint"]
    print(f"[i] {server} token expired -> refreshing via {endpoint}", file=sys.stderr)
    data = urllib.parse.urlencode({
        "grant_type": "refresh_token",
        "refresh_token": tok["refresh_token"],
        "client_id": client["client_id"],
    }).encode()
    req = urllib.request.Request(endpoint, data=data,
                                 headers={"Content-Type": "application/x-www-form-urlencoded",
                                          "Accept": "application/json",
                                          "User-Agent": UA})
    with urllib.request.urlopen(req, timeout=30) as r:
        resp = json.load(r)
    new = dict(tok)
    new.update(resp)
    if "refresh_token" not in resp:
        new["refresh_token"] = tok["refresh_token"]
    new["expires_at"] = time.time() + resp.get("expires_in", 3600)
    with open(f"{PROFILE}/mcp-tokens/{server}.json", "w") as f:
        json.dump(new, f)
    return new["access_token"]

SESSION_ID = None

def parse_sse(raw):
    out = []
    for block in raw.split("\n\n"):
        data_lines = [l[5:].strip() for l in block.splitlines() if l.startswith("data:")]
        if data_lines:
            try:
                out.append(json.loads("".join(data_lines)))
            except Exception:
                pass
    return out

def mcp_call(base, token, method, params=None):
    global SESSION_ID
    body = {"jsonrpc": "2.0", "method": method, "id": 1}
    if params is not None:
        body["params"] = params
    headers = {
        "Content-Type": "application/json",
        "Accept": "application/json, text/event-stream",
        "Authorization": "Bearer " + token,
        "User-Agent": UA,
        "X-User-Agent": UA,
    }
    if SESSION_ID:
        headers["Mcp-Session-Id"] = SESSION_ID
    req = urllib.request.Request(base, data=json.dumps(body).encode(), headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=90) as r:
            sid = r.headers.get("Mcp-Session-Id")
            if sid:
                SESSION_ID = sid
            ctype = r.headers.get("Content-Type", "")
            raw = r.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8", "replace")
        try:
            j = json.loads(raw)
            if "jsonrpc" in j:
                return j
        except Exception:
            pass
        raise RuntimeError(f"HTTP {e.code} {e.reason}: {raw[:500]}")
    if not raw.strip():
        return None
    if "text/event-stream" in ctype:
        msgs = parse_sse(raw)
        for m in msgs:
            if m.get("id") == 1 or "result" in m or "error" in m:
                return m
        return msgs[0] if msgs else None
    try:
        return json.loads(raw)
    except Exception:
        msgs = parse_sse(raw)
        return msgs[0] if msgs else {"raw": raw[:500]}

def tool_call(base, token, name, args):
    resp = mcp_call(base, token, "tools/call", {"name": name, "arguments": args})
    if resp is None:
        return {"isError": True, "text": "empty response"}
    if resp.get("error"):
        return {"isError": True, "text": json.dumps(resp["error"])}
    r = resp.get("result", {})
    out = {"isError": r.get("isError", False)}
    texts = [c.get("text", "") for c in r.get("content", []) if c.get("type") == "text"]
    out["text"] = "\n".join(texts)
    return out

def main():
    if len(sys.argv) < 3:
        print(__doc__)
        sys.exit(1)
    server, mode = sys.argv[1], sys.argv[2]
    if server not in SERVERS:
        print(f"unknown server {server}; known: {list(SERVERS)}")
        sys.exit(1)
    base = SERVERS[server]
    tok, client, meta = load(server)
    token = get_access_token(server, tok, client, meta)
    init = mcp_call(base, token, "initialize", {
        "protocolVersion": "2025-03-26",
        "capabilities": {},
        "clientInfo": {"name": "katsironni-build", "version": "1.0"},
    })
    if init and init.get("result", {}).get("serverInfo"):
        si = init["result"]["serverInfo"]
        print(f"[init] {si.get('name')} v{si.get('version')}", file=sys.stderr)
    try:
        mcp_call(base, token, "notifications/initialized")
    except Exception:
        pass
    if mode == "tools":
        tools = mcp_call(base, token, "tools/list", {})
        tl = tools.get("result", {}).get("tools", [])
        for t in tl:
            print("-", t.get("name"))
        print(f"[{len(tl)} tools]", file=sys.stderr)
        return
    if mode == "call":
        if len(sys.argv) < 4:
            print("call needs <tool> [json-args]")
            sys.exit(1)
        name = sys.argv[3]
        args = json.loads(sys.argv[4]) if len(sys.argv) > 4 else {}
        out = tool_call(base, token, name, args)
        print(out.get("text", json.dumps(out)))
        sys.exit(2 if out.get("isError") else 0)
    print(__doc__)

if __name__ == "__main__":
    main()
