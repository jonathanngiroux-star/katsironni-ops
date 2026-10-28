package email

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"katsironni-hub/internal/config"
	"katsironni-hub/internal/db"
)

type Client struct {
	apiKey string
	from   string
	client *http.Client
}

type EmailRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Html    string   `json:"html"`
	Text    string   `json:"text,omitempty"`
}

type EmailResponse struct {
	ID string `json:"id"`
}

func NewClient(cfg *config.Config) *Client {
	return &Client{
		apiKey: cfg.ResendAPIKey,
		from:   "Katsironni <hello@katsironni.com>",
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) Send(to, subject, html, text string) error {
	if c.apiKey == "" {
		return fmt.Errorf("Resend API key not configured")
	}

	req := EmailRequest{
		From:    c.from,
		To:      []string{to},
		Subject: subject,
		Html:    html,
		Text:    text,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewReader(data))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("resend error: %d", resp.StatusCode)
	}

	return nil
}

func (c *Client) SendVerificationEmail(to, token string) error {
	verifyURL := fmt.Sprintf("https://katsironni.com/verify-email?token=%s", token)
	html := fmt.Sprintf(`
		<h1>Verify your email</h1>
		<p>Welcome to Katsironni! Please verify your email address by clicking the link below:</p>
		<p><a href="%s">%s</a></p>
		<p>This link expires in 24 hours.</p>
	`, verifyURL, verifyURL)
	text := fmt.Sprintf("Verify your email: %s\nThis link expires in 24 hours.", verifyURL)
	return c.Send(to, "Verify your Katsironni account", html, text)
}

func (c *Client) SendPaymentReceipt(to, serviceName, interval string, amountCents int, method string) error {
	amount := fmt.Sprintf("$%.2f", float64(amountCents)/100)
	html := fmt.Sprintf(`
		<h1>Payment Receipt</h1>
		<p>Thank you for your payment!</p>
		<ul>
			<li>Service: %s</li>
			<li>Plan: %s</li>
			<li>Amount: %s CAD</li>
			<li>Method: %s</li>
		</ul>
		<p>Your subscription is now active.</p>
	`, serviceName, interval, amount, method)
	text := fmt.Sprintf("Payment Receipt: %s %s - %s CAD via %s", serviceName, interval, amount, method)
	return c.Send(to, "Payment Receipt - Katsironni", html, text)
}

func (c *Client) SendExpiryWarning(to, serviceName string, daysUntilExpiry int) error {
	html := fmt.Sprintf(`
		<h1>Subscription Expiring Soon</h1>
		<p>Your %s subscription expires in %d days.</p>
		<p><a href="https://katsironni.com/account">Renew now</a> to avoid service interruption.</p>
	`, serviceName, daysUntilExpiry)
	text := fmt.Sprintf("Your %s subscription expires in %d days. Renew at https://katsironni.com/account", serviceName, daysUntilExpiry)
	return c.Send(to, "Subscription Expiring Soon - Katsironni", html, text)
}

func (c *Client) SendCryptoVerified(to, serviceName, txHash string) error {
	html := fmt.Sprintf(`
		<h1>Crypto Payment Verified</h1>
		<p>Your crypto payment for %s has been verified on-chain.</p>
		<p>Transaction: <code>%s</code></p>
		<p>Your subscription is now active.</p>
	`, serviceName, txHash)
	text := fmt.Sprintf("Crypto payment verified for %s. Transaction: %s", serviceName, txHash)
	return c.Send(to, "Crypto Payment Verified - Katsironni", html, text)
}

type EmailQueue struct {
	db *db.DB
	client *Client
}

func NewEmailQueue(database *db.DB, cfg *config.Config) *EmailQueue {
	return &EmailQueue{
		db:     database,
		client: NewClient(cfg),
	}
}

func (q *EmailQueue) QueueEmail(to, subject, html, text, purpose string) error {
	// For now, send immediately. In production, add to queue table.
	return q.client.Send(to, subject, html, text)
}

func (q *EmailQueue) ProcessQueue() error {
	// Process queued emails - placeholder for future implementation
	return nil
}