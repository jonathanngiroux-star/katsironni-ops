package http

// Chat WebSocket endpoint, free-tier preview counting, file uploads, and the
// host-dispatched {service}.katsironni.com pages for the per-service agent chat.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"katsironni-hub/internal/config"
)

// serviceFromHost returns the service key when the request arrives on a
// {service}.katsironni.com host (apex/www/mcp/agentmail excluded).
func serviceFromHost(host string) string {
	host = strings.ToLower(strings.Split(host, ":")[0])
	host = strings.TrimSuffix(host, ".")
	if !strings.HasSuffix(host, ".katsironni.com") {
		return ""
	}
	sub := strings.TrimSuffix(host, ".katsironni.com")
	if sub == "" || sub == "www" || strings.HasPrefix(sub, "mcp.") || sub == "agentmail" {
		return ""
	}
	// Known chat-service keys only (the original 6 keep their app UIs).
	if _, ok := config.ServicePages[sub]; ok {
		return sub
	}
	return ""
}

// serviceLandingPage renders {service}.katsironni.com — explanation + chat.
func (s *Server) serviceLandingPage(c *gin.Context, serviceKey string) {
	product, ok := config.GetProduct(serviceKey)
	if !ok {
		c.HTML(http.StatusNotFound, "404.html", nil)
		return
	}
	page := config.ServicePages[serviceKey]

	subscribed := false
	if cookie, err := c.Cookie("hub_session"); err == nil {
		if userID, err := s.auth.ValidateSession(cookie); err == nil {
			subscribed, _ = s.entitlement.HasActiveEntitlement(userID, serviceKey)
		}
	}

	c.HTML(http.StatusOK, "service.html", gin.H{
		"Product":    product,
		"Page":       page,
		"Subscribed": subscribed,
		"PublicURL":  s.cfg.PublicURL,
	})
}

const freePreviewMessages = 2
const maxUploadBytes = 10 << 20 // 10 MB

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  16 << 10,
	WriteBufferSize: 16 << 10,
	CheckOrigin: func(r *http.Request) bool {
		// same-site only: the chat lives on *.katsironni.com behind Caddy
		host := strings.ToLower(r.Host)
		return strings.HasSuffix(host, "katsironni.com") || host == "localhost:8080" || strings.HasPrefix(host, "localhost")
	},
}

// chatUsageMonth returns this month's message count for user+service.
func (s *Server) chatUsageMonth(userID, serviceKey string) (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT message_count FROM chat_usage WHERE user_id = ? AND service_key = ? AND month = ?`,
		userID, serviceKey, time.Now().UTC().Format("2006-01"),
	).Scan(&count)
	if err != nil {
		return 0, nil // no row yet
	}
	return count, nil
}

func (s *Server) bumpChatUsage(userID, serviceKey string) error {
	_, err := s.db.Exec(
		`INSERT INTO chat_usage (user_id, service_key, month, message_count) VALUES (?, ?, ?, 1)
		 ON CONFLICT(user_id, service_key, month) DO UPDATE SET message_count = message_count + 1, updated_at = ?`,
		userID, serviceKey, time.Now().UTC().Format("2006-01"), time.Now().Format(time.RFC3339))
	return err
}

// chatUsageSummary powers the dashboard's per-service monthly usage.
func (s *Server) chatUsageSummary(userID string) map[string]int {
	out := map[string]int{}
	rows, err := s.db.Query(
		`SELECT service_key, message_count FROM chat_usage
		 WHERE user_id = ? AND month = ?`, userID, time.Now().UTC().Format("2006-01"))
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var svc string
		var n int
		if rows.Scan(&svc, &n) == nil {
			out[svc] = n
		}
	}
	return out
}

type wsInbound struct {
	Type  string   `json:"type"` // "message"
	Text  string   `json:"text"`
	Files []string `json:"files"` // upload IDs from /api/chat/{service}/upload
}

type wsOutbound struct {
	Type             string `json:"type"` // "agent" | "usage" | "error"
	Text             string `json:"text,omitempty"`
	RemainingPreview *int   `json:"remaining_preview,omitempty"`
	Subscribed       bool   `json:"subscribed"`
	Error            string `json:"error,omitempty"`
	SubscribePrompt  bool   `json:"subscribe_prompt,omitempty"`
}

func (s *Server) chatWS(c *gin.Context) {
	serviceKey := c.Param("service")
	if _, ok := config.GetProduct(serviceKey); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown service"})
		return
	}

	// auth via session cookie (same-origin WS handshake carries cookies)
	cookie, err := c.Cookie("hub_session")
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
		return
	}
	userID, err := s.auth.ValidateSession(cookie)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
		return
	}

	subscribed, _ := s.entitlement.HasActiveEntitlement(userID, serviceKey)

	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	sessionID := fmt.Sprintf("%s:%s", userID, serviceKey)

	// initial usage state
	count, _ := s.chatUsageMonth(userID, serviceKey)
	_ = s.wsSend(conn, wsOutbound{Type: "usage", Subscribed: subscribed, RemainingPreview: previewLeft(count, subscribed)})

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var in wsInbound
		if json.Unmarshal(raw, &in) != nil || in.Type != "message" || strings.TrimSpace(in.Text) == "" {
			continue
		}

		// free-tier gate
		count, _ := s.chatUsageMonth(userID, serviceKey)
		if !subscribed && count >= freePreviewMessages {
			_ = s.wsSend(conn, wsOutbound{
				Type: "error", Error: "preview limit reached",
				SubscribePrompt: true, Subscribed: false,
			})
			continue
		}

		// resolve upload IDs -> stored file paths
		var files []string
		for _, id := range in.Files {
			if p := s.uploadPath(userID, serviceKey, id); p != "" {
				files = append(files, p)
			}
		}

		_ = s.bumpChatUsage(userID, serviceKey)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		reply, rerr := s.chatManager.Respond(ctx, serviceKey, userID, sessionID, in.Text, files)
		cancel()
		if rerr != nil {
			_ = s.wsSend(conn, wsOutbound{Type: "error", Error: "agent unavailable — try again shortly", Subscribed: subscribed})
			continue
		}

		count, _ = s.chatUsageMonth(userID, serviceKey)
		_ = s.wsSend(conn, wsOutbound{Type: "agent", Text: reply, Subscribed: subscribed, RemainingPreview: previewLeft(count, subscribed)})
	}
}

func previewLeft(count int, subscribed bool) *int {
	if subscribed {
		return nil // unlimited — don't render a counter
	}
	left := freePreviewMessages - count
	if left < 0 {
		left = 0
	}
	return &left
}

func (s *Server) wsSend(conn *websocket.Conn, msg wsOutbound) error {
	return conn.WriteJSON(msg)
}

// ---- uploads ----

var uploadAllowlist = map[string]bool{
	".txt": true, ".md": true, ".json": true, ".csv": true, ".log": true,
	".pdf": true, ".png": true, ".jpg": true, ".jpeg": true, ".sol": true,
	".go": true, ".py": true, ".ts": true, ".js": true, ".yaml": true, ".yml": true, ".zip": true,
}

func (s *Server) uploadPath(userID, serviceKey, id string) string {
	if id == "" || strings.Contains(id, "/") || strings.Contains(id, "..") {
		return ""
	}
	return filepath.Join(s.workspaceRoot(), serviceKey, "uploads", userID, id)
}

func (s *Server) workspaceRoot() string {
	if r := os.Getenv("AGENT_WORKSPACES"); r != "" {
		return r
	}
	return "/opt/katsironni/agent-workspaces"
}

func (s *Server) chatUpload(c *gin.Context) {
	serviceKey := c.Param("service")
	if _, ok := config.GetProduct(serviceKey); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown service"})
		return
	}
	cookie, err := c.Cookie("hub_session")
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
		return
	}
	userID, err := s.auth.ValidateSession(cookie)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
		return
	}

	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file field required"})
		return
	}
	if fh.Size > maxUploadBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "max 10MB"})
		return
	}
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if !uploadAllowlist[ext] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file type not allowed"})
		return
	}

	nb := make([]byte, 8)
	rand.Read(nb)
	id := hex.EncodeToString(nb) + ext
	dir := filepath.Join(s.workspaceRoot(), serviceKey, "uploads", userID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "storage failed"})
		return
	}
	src, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "read failed"})
		return
	}
	defer src.Close()
	dst, err := os.OpenFile(filepath.Join(dir, id), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "storage failed"})
		return
	}
	defer dst.Close()
	if _, err := io.Copy(dst, io.LimitReader(src, maxUploadBytes+1)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "write failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "name": fh.Filename})
}
