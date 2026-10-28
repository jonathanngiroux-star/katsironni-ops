# Service-Agent Deployment Runbook (per knowledge-work service)

When @github's skill packages arrive, each V2 knowledge service gets a Hermes agent profile
on the VPS. Proven pattern (tested 2026-10-26 with test-agent):

```bash
# On the VPS, as the katsironni user (the hub runs as this user):
runuser -u katsironni -- bash -lc '
  hermes profile create <service>-bot &&
  grep -E "^(NVIDIA_API_KEY|OPENROUTER_API_KEY)=" /opt/katsironni/.hermes/.env \
    >> /opt/katsironni/.hermes/profiles/<service>-bot/.env &&
  chmod 600 /opt/katsironni/.hermes/profiles/<service>-bot/.env &&
  # install the service skill package into the profile (from @github repo):
  cp -r <repo>/skills/<skill> /opt/katsironni/.hermes/profiles/<service>-bot/skills/ 2>/dev/null || true
'
```

Key facts learned (do not rediscover):
- Profiles have their OWN .env — keys do NOT inherit from the main /opt/katsironni/.hermes/.env;
  copy the two API keys into every profile .env (chmod 600).
- Model config: profile inherits the main config unless overridden. Current main config:
  primary = minimaxai/minimax-m3.1-flash via NVIDIA NIM; fallback = z-ai/glm-5.3-flash via OpenRouter.
  (NIM's z-ai + deepseek models hang from ALL networks as of 2026-10-26 — NIM-side issue;
  when it recovers: `hermes config set model.default z-ai/glm-5.3-flash` restores the user's
  originally requested arrangement.)
- Test a profile: `hermes -p <service>-bot chat -q "..."` — expect a reply in ~1-8s.
- The hub's chat backend wires a service to its profile via config (chat backend interface,
  internal/chat/chat.go). EchoBackend is live today; a HermesProfileBackend execs
  `hermes -p <service>-bot chat -q` per message — implement when skills land (5-min patch).
- File uploads already land in /opt/katsironni/agent-workspaces/<service>/uploads/<user>/ —
  pass the paths in the backend call for the agent to read.
