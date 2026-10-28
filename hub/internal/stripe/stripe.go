package stripe

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"katsironni-hub/internal/config"
	"katsironni-hub/internal/db"
	"katsironni-hub/internal/entitlement"
)

type Client struct {
	secretKey     string
	webhookSecret string
	httpClient    *http.Client
	cfg           *config.Config
	db            *db.DB
}

type CheckoutSessionRequest struct {
	SuccessURL string            `json:"success_url"`
	CancelURL  string            `json:"cancel_url"`
	Mode       string            `json:"mode"` // "subscription"
	CustomerEmail string         `json:"customer_email,omitempty"`
	LineItems  []LineItem        `json:"line_items"`
	Metadata   map[string]string `json:"metadata"`
}

type LineItem struct {
	Price    string `json:"price"`
	Quantity int    `json:"quantity"`
}

type CheckoutSessionResponse struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Status string `json:"status"`
}

type WebhookEvent struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	APIVersion string         `json:"api_version"`
	Data      json.RawMessage `json:"data"`
	Created   int64           `json:"created"`
}

func NewClient(cfg *config.Config, database *db.DB) *Client {
	return &Client{
		secretKey:     cfg.StripeSecretKey,
		webhookSecret: cfg.StripeWebhookSecret,
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		cfg:           cfg,
		db:            database,
	}
}

func (c *Client) CreateCheckoutSession(serviceKey, interval, userEmail, userID string) (*CheckoutSessionResponse, error) {
	product, ok := config.GetProduct(serviceKey)
	if !ok {
		return nil, fmt.Errorf("unknown product: %s", serviceKey)
	}
	if !product.Live {
		return nil, fmt.Errorf("service not yet live: %s", serviceKey)
	}

	var priceID string
	mode := "subscription"
	switch interval {
	case "month":
		priceID = product.StripeMonthlyPriceID
	case "year":
		priceID = product.StripeAnnualPriceID
	case "onetime":
		priceID = product.StripeOnetimePriceID
		mode = "payment"
		if priceID == "" {
			return nil, fmt.Errorf("no one-time tier for %s", serviceKey)
		}
	default:
		return nil, fmt.Errorf("invalid interval: %s", interval)
	}

	if priceID == "" {
		return nil, fmt.Errorf("stripe price ID not configured for %s/%s", serviceKey, interval)
	}

	formData := url.Values{}
	formData.Set("success_url", c.cfg.PublicURL+"/account?checkout=success")
	formData.Set("cancel_url", c.cfg.PublicURL+"/p/"+serviceKey+"?checkout=canceled")
	formData.Set("mode", mode)
	formData.Set("customer_email", userEmail)
	formData.Set("line_items[0][price]", priceID)
	formData.Set("line_items[0][quantity]", "1")
	formData.Set("metadata[user_id]", userID)
	formData.Set("metadata[service_key]", serviceKey)
	formData.Set("metadata[interval]", interval)
	formData.Set("metadata[source]", "stripe")

	httpReq, err := http.NewRequest("POST", "https://api.stripe.com/v1/checkout/sessions", strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Authorization", "Bearer "+c.secretKey)
	httpReq.Header.Set("Stripe-Version", "2024-06-20")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("stripe error: %s", string(body))
	}

	var session CheckoutSessionResponse
	if err := json.Unmarshal(body, &session); err != nil {
		return nil, err
	}

	return &session, nil
}

func (c *Client) VerifyWebhookSignature(payload []byte, signature string) bool {
	// Stripe-Signature: t=timestamp,v1=signature
	parts := strings.Split(signature, ",")
	var timestamp, sig string
	for _, part := range parts {
		if strings.HasPrefix(part, "t=") {
			timestamp = part[2:]
		} else if strings.HasPrefix(part, "v1=") {
			sig = part[3:]
		}
	}
	if timestamp == "" || sig == "" {
		return false
	}

	// Check timestamp is recent (within 5 minutes)
	ts := timestamp
	tsInt := 0
	fmt.Sscanf(ts, "%d", &tsInt)
	if time.Now().Unix()-int64(tsInt) > 300 {
		return false
	}

	signedPayload := timestamp + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(c.webhookSecret))
	mac.Write([]byte(signedPayload))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expectedSig), []byte(sig))
}

func (c *Client) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	signature := r.Header.Get("Stripe-Signature")
	if signature == "" {
		http.Error(w, "Missing Stripe-Signature header", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	if !c.VerifyWebhookSignature(body, signature) {
		http.Error(w, "Invalid signature", http.StatusBadRequest)
		return
	}

	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Idempotency: check if event already processed
	var exists bool
	c.db.QueryRow(`SELECT 1 FROM stripe_events WHERE event_id = ?`, event.ID).Scan(&exists)
	if exists {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Process event
	err = c.processEvent(event)
	if err != nil {
		http.Error(w, "Event processing failed", http.StatusInternalServerError)
		return
	}

	// Record event
	c.db.Exec(`INSERT INTO stripe_events (id, event_id, type, processed_at) VALUES (?, ?, ?, ?)`,
		db.GenerateID(), event.ID, event.Type, time.Now().Format(time.RFC3339))

	w.WriteHeader(http.StatusOK)
}

func (c *Client) processEvent(event WebhookEvent) error {
	switch event.Type {
	case "checkout.session.completed":
		return c.handleCheckoutCompleted(event.Data)
	case "customer.subscription.updated":
		return c.handleSubscriptionUpdated(event.Data)
	case "customer.subscription.deleted":
		return c.handleSubscriptionDeleted(event.Data)
	case "invoice.payment_failed":
		return c.handleInvoicePaymentFailed(event.Data)
	case "invoice.paid":
		return c.handleInvoicePaid(event.Data)
	}
	return nil
}

func (c *Client) handleCheckoutCompleted(data json.RawMessage) error {
	var session struct {
		Customer          string            `json:"customer"`
		Subscription      string            `json:"subscription"`
		Metadata          map[string]string `json:"metadata"`
		PaymentStatus     string            `json:"payment_status"`
	}
	if err := json.Unmarshal(data, &session); err != nil {
		return err
	}

	if session.PaymentStatus != "paid" {
		return nil
	}

	userID := session.Metadata["user_id"]
	serviceKey := session.Metadata["service_key"]
	interval := session.Metadata["interval"]

	if userID == "" || serviceKey == "" || interval == "" {
		return fmt.Errorf("missing metadata")
	}

	now := time.Now()
	var periodEnd time.Time
	entInterval := interval
	switch interval {
	case "month":
		periodEnd = now.AddDate(0, 1, 0)
	case "onetime":
		entInterval = "lifetime"
		periodEnd = now.AddDate(100, 0, 0)
	default:
		periodEnd = now.AddDate(1, 0, 0)
	}

	e := &entitlement.Entitlement{
		ID:                   db.GenerateID(),
		UserID:               userID,
		ServiceKey:           serviceKey,
		Source:               "stripe",
		Status:               "active",
		Interval:             entInterval,
		PeriodStart:          now,
		PeriodEnd:            periodEnd,
		StripeCustomerID:     session.Customer,
		StripeSubscriptionID: session.Subscription,
		UpdatedAt:            now,
	}

	if err := entitlement.CreateOrUpdateEntitlement(c.db, e); err != nil {
		return err
	}

	// Bundles activate entitlements for every member service (config.BundleMembers).
	for _, svc := range config.BundleMembers[serviceKey] {
		m := *e
		m.ID = db.GenerateID()
		m.ServiceKey = svc
		m.Source = "bundle"
		m.StripeSubscriptionID = ""
		if err := entitlement.CreateOrUpdateEntitlement(c.db, &m); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) handleSubscriptionUpdated(data json.RawMessage) error {
	var sub struct {
		Customer          string            `json:"customer"`
		Subscription      string            `json:"id"`
		Status            string            `json:"status"`
		CurrentPeriodEnd  int64             `json:"current_period_end"`
		Metadata          map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(data, &sub); err != nil {
		return err
	}

	// Find entitlement by subscription ID
	var entID string
	err := c.db.QueryRow(
		`SELECT id FROM entitlements WHERE stripe_subscription_id = ?`, sub.Subscription,
	).Scan(&entID)
	if err != nil {
		return err
	}

	status := "active"
	if sub.Status == "past_due" {
		status = "past_due"
	} else if sub.Status == "canceled" {
		status = "canceled"
	}

	periodEnd := time.Unix(sub.CurrentPeriodEnd, 0)
	_, err = c.db.Exec(
		`UPDATE entitlements SET status = ?, period_end = ?, updated_at = ? WHERE id = ?`,
		status, periodEnd.Format(time.RFC3339), time.Now().Format(time.RFC3339), entID,
	)
	return err
}

func (c *Client) handleSubscriptionDeleted(data json.RawMessage) error {
	var sub struct {
		Subscription string `json:"id"`
	}
	if err := json.Unmarshal(data, &sub); err != nil {
		return err
	}

	_, err := c.db.Exec(
		`UPDATE entitlements SET status = 'canceled', updated_at = ? WHERE stripe_subscription_id = ?`,
		time.Now().Format(time.RFC3339), sub.Subscription,
	)
	return err
}

func (c *Client) handleInvoicePaymentFailed(data json.RawMessage) error {
	var inv struct {
		Subscription string `json:"subscription"`
		Customer     string `json:"customer"`
	}
	if err := json.Unmarshal(data, &inv); err != nil {
		return err
	}

	_, err := c.db.Exec(
		`UPDATE entitlements SET status = 'past_due', updated_at = ? WHERE stripe_subscription_id = ?`,
		time.Now().Format(time.RFC3339), inv.Subscription,
	)
	return err
}

func (c *Client) handleInvoicePaid(data json.RawMessage) error {
	var inv struct {
		Subscription string `json:"subscription"`
	}
	if err := json.Unmarshal(data, &inv); err != nil {
		return err
	}

	_, err := c.db.Exec(
		`UPDATE entitlements SET status = 'active', updated_at = ? WHERE stripe_subscription_id = ?`,
		time.Now().Format(time.RFC3339), inv.Subscription,
	)
	return err
}