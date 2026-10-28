package chat

// HermesProfileBackend runs a per-service Hermes agent profile as the chat
// backend. Each message becomes a one-shot `hermes -p <profile> chat -q`
// invocation with the session history and attached-file paths folded into the
// prompt. Deployed for services whose real agents are ready (e.g. mirofish).

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const hermesTimeout = 10 * time.Minute

// ProfileMap: service key -> Hermes profile name.
var ProfileMap = map[string]string{
	"mirofish": "mirofish-bot",
	"techdocs": "techdocs-bot",
	"regresearch": "regresearch-bot",
	"datasetcur": "datasetcur-bot",
	"finetune": "finetune-bot",
	"daogov": "daogov-bot",
	"tokenlaunch": "tokenlaunch-bot",
	"meshops": "meshops-bot",
	"keymgmt": "keymgmt-bot",
	"emailops": "emailops-bot",
}

type HermesProfileBackend struct {
	Profile string
}

func (h *HermesProfileBackend) Respond(ctx context.Context, serviceKey, userID, sessionID string, history []Message, files []string) (string, error) {
	var prompt strings.Builder
	if len(history) > 0 {
		prompt.WriteString("Conversation so far (for context):\n")
		const maxCtx = 12
		start := 0
		if len(history) > maxCtx {
			start = len(history) - maxCtx
		}
		for _, m := range history[start:] {
			role := "Client"
			if m.Role == "agent" {
				role = "You"
			}
			prompt.WriteString(role + ": " + clip(m.Text, 400) + "\n")
		}
		prompt.WriteString("\n")
	}
	last := ""
	if len(history) > 0 {
		last = history[len(history)-1].Text
	}
	prompt.WriteString("The client's latest message: " + last + "\n")
	if len(files) > 0 {
		prompt.WriteString("\nAttached files (read them from disk):\n")
		for _, f := range files {
			prompt.WriteString("- " + f + "\n")
		}
	}
	prompt.WriteString("\nRespond directly to the client in markdown. Be concise and actionable.")

	ctx2, cancel := context.WithTimeout(ctx, hermesTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx2, "runuser", "-u", "katsironni", "--", "bash", "-lc",
		fmt.Sprintf("hermes -p %s chat -q %q", h.Profile, prompt.String()))
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		tail := strings.TrimSpace(errb.String())
		if tail != "" {
			return "", fmt.Errorf("agent error: %s", clip(tail, 200))
		}
		return "", err
	}
	resp := strings.TrimSpace(out.String())
	if resp == "" {
		return "", fmt.Errorf("empty agent response")
	}
	// One-shot output includes the "Query:" echo and summary block; keep the reply body.
	if i := strings.Index(resp, "\n"); i > 0 && strings.HasPrefix(resp, "Query:") {
		resp = resp[i+1:]
	}
	// Trim trailing summary metadata if present.
	for _, marker := range []string{"\nSession:", "\nTitle:", "\nDuration:", "\nMessages:"} {
		if i := strings.Index(resp, marker); i > 0 {
			resp = resp[:i]
		}
	}
	return strings.TrimSpace(resp), nil
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// WireProfileBackends registers Hermes profile backends for configured services.
func (m *Manager) WireProfileBackends() {
	for svc, profile := range ProfileMap {
		m.SetBackend(svc, &HermesProfileBackend{Profile: profile})
	}
}
