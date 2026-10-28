package chat

// Chat backend abstraction for the per-service agent chat. The hub owns the WS
// transport, auth, free-tier counting, and file storage; backends only produce
// responses. EchoBackend ships until the real Hermes agent profiles deploy
// (@github's skill packages), at which point chat.backends config wires each
// service to its agent process.

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type Message struct {
	Role      string    `json:"role"` // "user" | "agent"
	Text      string    `json:"text"`
	At        time.Time `json:"at"`
}

type Backend interface {
	// Respond produces the agent's reply for a user message. history is the
	// session's prior turns (newest last). files are absolute paths of files
	// the user attached (already stored in the service workspace).
	Respond(ctx context.Context, serviceKey, userID, sessionID string, history []Message, files []string) (string, error)
}

// EchoBackend acknowledges messages until a real agent is wired. It still
// formats context (attached files, preview state) so the UX is testable now.
type EchoBackend struct {
	Notes map[string]string // per-service deployment note (optional)
}

func (e *EchoBackend) Respond(ctx context.Context, serviceKey, userID, sessionID string, history []Message, files []string) (string, error) {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("👋 This is the **%s** agent.\n\n", serviceKey))
	if note, ok := e.Notes[serviceKey]; ok && note != "" {
		b.WriteString(note + "\n\n")
	}
	b.WriteString("I received your message")
	if len(history) > 0 {
		b.WriteString(fmt.Sprintf(" (turn %d in this session)", len(history)/2+1))
	}
	b.WriteString(". ")
	if len(files) > 0 {
		names := make([]string, len(files))
		for i, f := range files {
			parts := strings.Split(f, "/")
			names[i] = parts[len(parts)-1]
		}
		b.WriteString(fmt.Sprintf("Attached files: %s. ", strings.Join(names, ", ")))
	}
	b.WriteString("\n\n> 🔧 The full agent engine for this service is being deployed. Your session, entitlement, and previews are fully live — once the agent is wired, this exact chat runs the real service.")
	return b.String(), nil
}

// Manager tracks per-session history and picks the backend per service.
type Manager struct {
	mu        sync.Mutex
	histories map[string][]Message // sessionID -> turns (capped)
	backends  map[string]Backend    // serviceKey -> backend (nil = echo)
	echo      *EchoBackend
}

func NewManager() *Manager {
	return &Manager{
		histories: map[string][]Message{},
		backends:  map[string]Backend{},
		echo:      &EchoBackend{},
	}
}

// SetBackend wires a real backend for a service (called from config at boot,
// or later when agents deploy).
func (m *Manager) SetBackend(serviceKey string, b Backend) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.backends[serviceKey] = b
}

const maxHistoryPerSession = 40

func (m *Manager) History(sessionID string) []Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Message{}, m.histories[sessionID]...)
}

// Respond routes to the service backend, records the turn, returns the reply.
func (m *Manager) Respond(ctx context.Context, serviceKey, userID, sessionID, text string, files []string) (string, error) {
	m.mu.Lock()
	hist := append([]Message{}, m.histories[sessionID]...)
	m.histories[sessionID] = append(m.histories[sessionID], Message{Role: "user", Text: text, At: time.Now()})
	if len(m.histories[sessionID]) > maxHistoryPerSession {
		m.histories[sessionID] = m.histories[sessionID][len(m.histories[sessionID])-maxHistoryPerSession:]
	}
	backend, ok := m.backends[serviceKey]
	m.mu.Unlock()

	if !ok || backend == nil {
		backend = m.echo
	}
	reply, err := backend.Respond(ctx, serviceKey, userID, sessionID, hist, files)
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	m.histories[sessionID] = append(m.histories[sessionID], Message{Role: "agent", Text: reply, At: time.Now()})
	m.mu.Unlock()
	return reply, nil
}
