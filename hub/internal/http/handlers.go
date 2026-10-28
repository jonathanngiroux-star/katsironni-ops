package http

import (
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"katsironni-hub/internal/auth"
	"katsironni-hub/internal/chat"
	"katsironni-hub/internal/config"
	"katsironni-hub/internal/crypto"
	"katsironni-hub/internal/db"
	"katsironni-hub/internal/email"
	"katsironni-hub/internal/entitlement"
	"katsironni-hub/internal/stripe"
)

// cad formats a cent amount as a decimal string (no currency symbol).
func cad(v any) string {
	switch n := v.(type) {
	case int64:
		return fmt.Sprintf("%.2f", float64(n)/100)
	case int:
		return fmt.Sprintf("%.2f", float64(n)/100)
	case float64:
		return fmt.Sprintf("%.2f", n/100)
	case uint64:
		return fmt.Sprintf("%.2f", float64(n)/100)
	}
	return "0.00"
}

type Server struct {
	db          *db.DB
	cfg         *config.Config
	auth        *auth.AuthService
	crypto      *crypto.VerificationService
	stripe      *stripe.Client
	email       *email.Client
	entitlement *entitlement.EntitlementService
	chatManager *chat.Manager
}

func NewServer(database *db.DB, cfg *config.Config) *Server {
	srv := &Server{
		db:          database,
		cfg:         cfg,
		auth:        auth.NewAuthService(database),
		crypto:      crypto.NewVerificationService(database, cfg.Chains),
		stripe:      stripe.NewClient(cfg, database),
		email:       email.NewClient(cfg),
		entitlement: entitlement.NewEntitlementService(database),
		chatManager: chat.NewManager(),
	}
	srv.chatManager.WireProfileBackends()
	return srv
}

func (s *Server) SetupRoutes() *gin.Engine {
	r := gin.Default()
	r.SetTrustedProxies(nil)

	// Template helpers (html/template has no arithmetic builtins)
	funcs := template.FuncMap{
		"cad": cad,
		"default": func(def any, v any) any {
			switch val := v.(type) {
			case nil:
				return def
			case string:
				if val == "" {
					return def
				}
			}
			return v
		},
		"safeHTML": func(s any) template.HTML { return template.HTML(fmt.Sprint(s)) },
	}

	// Layout renderer: per-file template sets (base skeleton + page define blocks).
	// LoadHTMLGlob would merge all pages into one namespace where "content"
	// defines collide and every page renders empty.
	lr, lerr := newLayoutRenderer(funcs)
	if lerr != nil {
		panic("template load: " + lerr.Error())
	}
	r.HTMLRender = lr

	// Static files
	r.Static("/static", "./static")

	// Health check
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "db": "connected"})
	})

	// Public routes
	r.GET("/", s.landingPage)
	r.GET("/p/:service", s.productPage)
	r.GET("/pricing", s.pricingPage)
	r.GET("/signup", s.signupPage)
	r.GET("/login", s.loginPage)
	r.POST("/signup", s.handleSignup)
	r.POST("/login", s.handleLogin)
	r.GET("/verify-email", s.handleVerifyEmail)
	r.GET("/checkout/crypto/:service/:interval", s.cryptoCheckoutPage)

	// Social authentication (SIWE + OAuth)
	r.POST("/auth/siwe/nonce", s.siweNonce)
	r.POST("/auth/siwe/verify", s.siweVerify)
	r.GET("/auth/google/start", s.oauthStart)
	r.GET("/auth/google/callback", s.oauthCallback)
	r.GET("/auth/x/start", s.oauthStart)
	r.GET("/auth/x/callback", s.oauthCallback)
	r.GET("/auth/github/start", s.oauthStart)
	r.GET("/auth/github/callback", s.oauthCallback)

	// Authenticated routes
	authGroup := r.Group("/")
	authGroup.Use(s.authMiddleware())
	{
		authGroup.GET("/account", s.accountPage)
		authGroup.POST("/api/logout", s.handleLogout)
		authGroup.GET("/checkout/stripe/:service/:interval", s.handleStripeCheckout)
		authGroup.POST("/api/crypto/submit", s.handleCryptoSubmit)
		authGroup.GET("/api/crypto/status/:id", s.handleCryptoStatus)
		authGroup.POST("/api/keys/:service/generate", s.generateAPIKey)
		authGroup.POST("/api/keys/:service/revoke", s.revokeAPIKey)
		authGroup.GET("/api/keys/:service/state", s.apiKeyState)
	}

	// Webhooks
	r.POST("/webhooks/stripe", s.handleStripeWebhook)

	// Forward auth for service gating
	r.GET("/api/auth/check", s.forwardAuth)

	// API-key check for Caddy forward_auth on mcp.* subdomains (Bearer, public)
	r.GET("/api/keycheck", s.apiKeyCheck)

	// Agent chat (per-service pages)
	r.GET("/ws/chat/:service", s.chatWS)
	r.POST("/api/chat/:service/upload", s.chatUpload)

	// Admin routes
	adminGroup := r.Group("/admin")
	adminGroup.Use(s.authMiddleware(), s.adminMiddleware())
	{
		adminGroup.GET("", s.adminDashboard)
		adminGroup.GET("/users", s.adminUsers)
		adminGroup.GET("/entitlements", s.adminEntitlements)
		adminGroup.GET("/payments", s.adminPayments)
		adminGroup.POST("/crypto/:id/approve", s.adminApproveCrypto)
		adminGroup.POST("/crypto/:id/reject", s.adminRejectCrypto)
	}

	return r
}

func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		cookie, err := c.Cookie("hub_session")
		if err != nil {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		userID, err := s.auth.ValidateSession(cookie)
		if err != nil {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		c.Set("user_id", userID)
		c.Next()
	}
}

func (s *Server) adminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		var isAdmin bool
		err := s.db.QueryRow(`SELECT is_admin FROM users WHERE id = ?`, userID).Scan(&isAdmin)
		if err != nil || !isAdmin {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	}
}

// Page handlers
func (s *Server) landingPage(c *gin.Context) {
	// Host-based dispatch: {service}.katsironni.com serves that service's page.
	if svc := serviceFromHost(c.Request.Host); svc != "" {
		s.serviceLandingPage(c, svc)
		return
	}
	products := config.LiveProducts()
	icons := map[string]string{
		"proofspan": "🧪", "switchyard": "⚙️", "warmline": "📬", "oneprobe": "🩺",
		"walletrisk": "🕵️", "airdropintel": "🪂", "bundle": "📦", "certaintystack": "🎯",
	}
	for k, p := range config.ServicePages {
		if _, exists := icons[k]; !exists {
			icons[k] = p.Icon
		}
	}
	c.HTML(http.StatusOK, "landing.html", gin.H{
		"Products":     products,
		"ServiceIcons": icons,
		"PublicURL":    s.cfg.PublicURL,
	})
}

func (s *Server) productPage(c *gin.Context) {
	serviceKey := c.Param("service")
	product, ok := config.GetProduct(serviceKey)
	if !ok || !product.Live {
		c.HTML(http.StatusNotFound, "404.html", nil)
		return
	}

	chains := s.cfg.Chains.EnabledChains()
	c.HTML(http.StatusOK, "product.html", gin.H{
		"Product":   product,
		"Chains":    chains,
		"PublicURL": s.cfg.PublicURL,
	})
}

func (s *Server) pricingPage(c *gin.Context) {
	products := config.LiveProducts()
	c.HTML(http.StatusOK, "pricing.html", gin.H{
		"Products": products,
	})
}

func (s *Server) signupPage(c *gin.Context) {
	c.HTML(http.StatusOK, "signup.html", gin.H{
		"PublicURL": s.cfg.PublicURL,
		"Social":    s.socialEnabled(),
	})
}

func (s *Server) loginPage(c *gin.Context) {
	c.HTML(http.StatusOK, "login.html", gin.H{
		"PublicURL": s.cfg.PublicURL,
		"Social":    s.socialEnabled(),
		"Error":     c.Query("error"),
		"Notice":    c.Query("notice"),
	})
}

func (s *Server) handleSignup(c *gin.Context) {
	email := strings.TrimSpace(c.PostForm("email"))
	password := c.PostForm("password")

	if email == "" || password == "" {
		c.HTML(http.StatusBadRequest, "signup.html", gin.H{"Error": "Email and password required"})
		return
	}

	// Check if user exists
	var exists bool
	s.db.QueryRow(`SELECT 1 FROM users WHERE email = ?`, email).Scan(&exists)
	if exists {
		c.HTML(http.StatusBadRequest, "signup.html", gin.H{"Error": "Email already registered"})
		return
	}

	// Hash password
	hash, err := auth.HashPassword(password)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "signup.html", gin.H{"Error": "Failed to create account"})
		return
	}

	userID := db.GenerateID()
	_, err = s.db.Exec(
		`INSERT INTO users (id, email, password_hash) VALUES (?, ?, ?)`,
		userID, email, hash,
	)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "signup.html", gin.H{"Error": "Failed to create account"})
		return
	}

	// Create verification token
	token, _ := auth.GenerateToken()
	tokenHash := auth.HashToken(token)
	expiresAt := time.Now().Add(24 * time.Hour)

	_, err = s.db.Exec(
		`INSERT INTO email_tokens (id, user_id, token, purpose, expires_at) VALUES (?, ?, ?, 'verify', ?)`,
		db.GenerateID(), userID, tokenHash, expiresAt.Format(time.RFC3339),
	)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "signup.html", gin.H{"Error": "Failed to send verification email"})
		return
	}

	// Send verification email
	s.email.SendVerificationEmail(email, token)

	c.HTML(http.StatusOK, "signup_success.html", gin.H{"Email": email})
}

func (s *Server) handleLogin(c *gin.Context) {
	email := strings.TrimSpace(c.PostForm("email"))
	password := c.PostForm("password")

	if email == "" || password == "" {
		c.HTML(http.StatusBadRequest, "login.html", gin.H{"Error": "Email and password required"})
		return
	}

	var userID, hash string
	var emailVerifiedAt sql.NullString
	err := s.db.QueryRow(
		`SELECT id, password_hash, email_verified_at FROM users WHERE email = ?`,
		email,
	).Scan(&userID, &hash, &emailVerifiedAt)
	if err != nil {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{"Error": "Invalid credentials"})
		return
	}

	valid, err := auth.VerifyPassword(password, hash)
	if err != nil || !valid {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{"Error": "Invalid credentials"})
		return
	}

	if !emailVerifiedAt.Valid {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{"Error": "Please verify your email first"})
		return
	}

	// Create session
	token, err := s.auth.CreateSession(userID)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "login.html", gin.H{"Error": "Failed to create session"})
		return
	}

	c.SetCookie("hub_session", token, int((30 * 24 * time.Hour).Seconds()), "/", ".katsironni.com", true, true)
	c.Redirect(http.StatusFound, "/account")
}

func (s *Server) handleVerifyEmail(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.HTML(http.StatusBadRequest, "verify.html", gin.H{"Error": "Invalid token"})
		return
	}

	tokenHash := auth.HashToken(token)
	var userID string
	var expiresAtStr string
	err := s.db.QueryRow(
		`SELECT user_id, expires_at FROM email_tokens WHERE token = ? AND purpose = 'verify'`,
		tokenHash,
	).Scan(&userID, &expiresAtStr)
	if err != nil {
		c.HTML(http.StatusBadRequest, "verify.html", gin.H{"Error": "Invalid or expired token"})
		return
	}

	expiresAt, _ := time.Parse(time.RFC3339, expiresAtStr)
	if time.Now().After(expiresAt) {
		c.HTML(http.StatusBadRequest, "verify.html", gin.H{"Error": "Token expired"})
		return
	}

	// Mark email as verified
	_, err = s.db.Exec(
		`UPDATE users SET email_verified_at = ? WHERE id = ?`,
		time.Now().Format(time.RFC3339), userID,
	)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "verify.html", gin.H{"Error": "Verification failed"})
		return
	}

	// Delete token
	s.db.Exec(`DELETE FROM email_tokens WHERE token = ?`, tokenHash)

	c.HTML(http.StatusOK, "verify.html", gin.H{"Success": true})
}

func (s *Server) handleStripeCheckout(c *gin.Context) {
	userID := c.GetString("user_id")
	serviceKey := c.Param("service")
	interval := c.Param("interval")

	var userEmail string
	s.db.QueryRow(`SELECT email FROM users WHERE id = ?`, userID).Scan(&userEmail)

	session, err := s.stripe.CreateCheckoutSession(serviceKey, interval, userEmail, userID)
	if err != nil {
		c.HTML(http.StatusOK, "crypto_fallback.html", gin.H{
			"ServiceKey": serviceKey,
			"Interval":   interval,
			"Message":    "Card checkout temporarily unavailable. Please use crypto payment.",
		})
		return
	}

	c.Redirect(http.StatusFound, session.URL)
}

func (s *Server) cryptoCheckoutPage(c *gin.Context) {
	serviceKey := c.Param("service")
	interval := c.Param("interval")

	product, ok := config.GetProduct(serviceKey)
	if !ok || !product.Live {
		c.HTML(http.StatusNotFound, "404.html", nil)
		return
	}

	chains := s.cfg.Chains.EnabledChains()
	c.HTML(http.StatusOK, "crypto_checkout.html", gin.H{
		"Product":   product,
		"Interval":  interval,
		"Chains":    chains,
		"PublicURL": s.cfg.PublicURL,
	})
}

func (s *Server) handleCryptoSubmit(c *gin.Context) {
	userID := c.GetString("user_id")

	var req struct {
		Chain      string `json:"chain" binding:"required"`
		TxHash     string `json:"txhash" binding:"required"`
		ServiceKey string `json:"service" binding:"required"`
		Interval   string `json:"interval" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	result, err := s.crypto.SubmitCryptoPayment(c.Request.Context(), userID, req.ServiceKey, req.Interval, req.Chain, req.TxHash)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (s *Server) handleCryptoStatus(c *gin.Context) {
	userID := c.GetString("user_id")
	paymentID := c.Param("id")

	payment, err := s.crypto.GetPaymentStatus(paymentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Payment not found"})
		return
	}

	if payment.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	c.JSON(http.StatusOK, payment)
}

func (s *Server) handleLogout(c *gin.Context) {
	cookie, err := c.Cookie("hub_session")
	if err == nil {
		s.auth.DeleteSession(cookie)
	}
	c.SetCookie("hub_session", "", -1, "/", ".katsironni.com", true, true)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) accountPage(c *gin.Context) {
	userID := c.GetString("user_id")

	entitlements, _ := s.entitlement.GetUserEntitlements(userID)
	cryptoPayments := []crypto.PaymentCrypto{} // TODO: fetch payments
	apiKeys, _ := s.auth.ListAPIKeys(userID)
	chatUsage := s.chatUsageSummary(userID)

	c.HTML(http.StatusOK, "account.html", gin.H{
		"Entitlements":   entitlements,
		"CryptoPayments": cryptoPayments,
		"APIKeys":        apiKeys,
		"ChatUsage":      chatUsage,
		"PublicURL":      s.cfg.PublicURL,
	})
}

func (s *Server) forwardAuth(c *gin.Context) {
	service := c.Query("service")
	if service == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	cookie, err := c.Cookie("hub_session")
	if err != nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	userID, err := s.auth.ValidateSession(cookie)
	if err != nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	hasAccess, err := s.entitlement.HasActiveEntitlement(userID, service)
	if err != nil || !hasAccess {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	c.Status(http.StatusOK)
}

// Admin handlers
func (s *Server) adminDashboard(c *gin.Context) {
	// Stats
	var userCount, activeSubs, pendingPayments int
	s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&userCount)
	s.db.QueryRow(`SELECT COUNT(*) FROM entitlements WHERE status = 'active' AND period_end > ?`, time.Now().Format(time.RFC3339)).Scan(&activeSubs)
	s.db.QueryRow(`SELECT COUNT(*) FROM payments_crypto WHERE status = 'pending'`).Scan(&pendingPayments)

	c.HTML(http.StatusOK, "admin.html", gin.H{
		"UserCount":       userCount,
		"ActiveSubs":      activeSubs,
		"PendingPayments": pendingPayments,
	})
}

func (s *Server) adminUsers(c *gin.Context) {
	rows, _ := s.db.Query(`SELECT id, email, email_verified_at, is_admin, created_at FROM users ORDER BY created_at DESC`)
	defer rows.Close()

	type User struct {
		ID              string
		Email           string
		EmailVerifiedAt sql.NullString
		IsAdmin         bool
		CreatedAt       string
	}
	var users []User
	for rows.Next() {
		var u User
		rows.Scan(&u.ID, &u.Email, &u.EmailVerifiedAt, &u.IsAdmin, &u.CreatedAt)
		users = append(users, u)
	}

	c.JSON(http.StatusOK, users)
}

func (s *Server) adminEntitlements(c *gin.Context) {
	rows, _ := s.db.Query(`
		SELECT e.id, u.email, e.service_key, e.source, e.status, e.interval, e.period_start, e.period_end
		FROM entitlements e JOIN users u ON e.user_id = u.id
		ORDER BY e.updated_at DESC
	`)
	defer rows.Close()

	type Ent struct {
		ID          string
		Email       string
		ServiceKey  string
		Source      string
		Status      string
		Interval    string
		PeriodStart string
		PeriodEnd   string
	}
	var ents []Ent
	for rows.Next() {
		var e Ent
		rows.Scan(&e.ID, &e.Email, &e.ServiceKey, &e.Source, &e.Status, &e.Interval, &e.PeriodStart, &e.PeriodEnd)
		ents = append(ents, e)
	}

	c.JSON(http.StatusOK, ents)
}

func (s *Server) adminPayments(c *gin.Context) {
	rows, _ := s.db.Query(`
		SELECT p.id, u.email, p.service_key, p.interval, p.chain, p.txhash, p.expected_cad_cents, p.status, p.submitted_at, p.verified_at
		FROM payments_crypto p JOIN users u ON p.user_id = u.id
		ORDER BY p.submitted_at DESC
	`)
	defer rows.Close()

	type Pay struct {
		ID               string
		Email            string
		ServiceKey       string
		Interval         string
		Chain            string
		TxHash           string
		ExpectedCADCents int
		Status           string
		SubmittedAt      string
		VerifiedAt       sql.NullString
	}
	var pays []Pay
	for rows.Next() {
		var p Pay
		rows.Scan(&p.ID, &p.Email, &p.ServiceKey, &p.Interval, &p.Chain, &p.TxHash, &p.ExpectedCADCents, &p.Status, &p.SubmittedAt, &p.VerifiedAt)
		pays = append(pays, p)
	}

	c.JSON(http.StatusOK, pays)
}

func (s *Server) adminApproveCrypto(c *gin.Context) {
	paymentID := c.Param("id")
	s.crypto.UpdatePaymentStatus(paymentID, "verified", "")
	s.crypto.ActivateEntitlement(paymentID)

	// Audit log
	s.db.Exec(`INSERT INTO audit_log (id, actor, action, detail_json) VALUES (?, ?, ?, ?)`,
		db.GenerateID(), c.GetString("user_id"), "crypto_approve", fmt.Sprintf(`{"payment_id": "%s"}`, paymentID))

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) adminRejectCrypto(c *gin.Context) {
	paymentID := c.Param("id")
	s.crypto.UpdatePaymentStatus(paymentID, "rejected", "admin_rejected")

	s.db.Exec(`INSERT INTO audit_log (id, actor, action, detail_json) VALUES (?, ?, ?, ?)`,
		db.GenerateID(), c.GetString("user_id"), "crypto_reject", fmt.Sprintf(`{"payment_id": "%s"}`, paymentID))

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) handleStripeWebhook(c *gin.Context) {
	// Convert gin context to http.ResponseWriter and *http.Request
	s.stripe.HandleWebhook(c.Writer, c.Request)
}
