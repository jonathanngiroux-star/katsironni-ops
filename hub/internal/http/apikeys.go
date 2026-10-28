package http

// API-key endpoints: Caddy-facing keycheck (Bearer, public) and the user-facing
// generate/revoke endpoints (session-authenticated).

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// apiKeyCheck is called by Caddy forward_auth for mcp.* subdomains:
// Authorization: Bearer kk_live_... + ?service=<key> → 200 when key + entitlement are live.
func (s *Server) apiKeyCheck(c *gin.Context) {
	service := c.Query("service")
	if service == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	authz := c.GetHeader("Authorization")
	if !strings.HasPrefix(authz, "Bearer ") {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	raw := strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))
	userID, err := s.auth.ValidateAPIKey(raw, service)
	if err != nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	has, err := s.entitlement.HasActiveEntitlement(userID, service)
	if err != nil || !has {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Header("X-User-Id", userID)
	c.Status(http.StatusOK)
}

func (s *Server) generateAPIKey(c *gin.Context) {
	userID := c.GetString("user_id")
	service := c.Param("service")
	has, err := s.entitlement.HasActiveEntitlement(userID, service)
	if err != nil || !has {
		c.JSON(http.StatusForbidden, gin.H{"error": "active subscription required for this service"})
		return
	}
	raw, err := s.auth.EnsureAPIKey(userID, service)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "key generation failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"key":   raw,
		"note":  "Copy this key now — it will not be shown again.",
	})
}

func (s *Server) revokeAPIKey(c *gin.Context) {
	userID := c.GetString("user_id")
	service := c.Param("service")
	if err := s.auth.RevokeAPIKey(userID, service); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// apiKeyState reports whether the user currently has an active key for a service
// (metadata only — never the raw key). Used by the account page.
func (s *Server) apiKeyState(c *gin.Context) {
	userID := c.GetString("user_id")
	service := c.Param("service")
	keys, err := s.auth.ListAPIKeys(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup failed"})
		return
	}
	for _, k := range keys {
		if k.ServiceKey == service && k.Status == "active" {
			c.JSON(http.StatusOK, gin.H{
				"active":      true,
				"created_at":  k.CreatedAt,
				"last_used_at": k.LastUsedAt,
			})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"active": false})
}
