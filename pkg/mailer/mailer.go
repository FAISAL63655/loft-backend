package mailer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// secrets holds Encore-managed secrets for Mailgun integration.
var secrets struct {
	MailgunAPIKey    string //encore:secret
	MailgunDomain    string //encore:secret
	MailgunFromEmail string //encore:secret
	MailgunFromName  string //encore:secret
}

// Client provides email sending via Mailgun HTTP API.
type Client struct {
	apiKey     string
	domain     string
	fromEmail  string
	fromName   string
	apiBaseURL string
}

// NewClient constructs a new Mailgun mailer client.
func NewClient() *Client {
	// Use EU region for European domains, US region for US domains
	// The domain will be appended in the Send method
	apiBase := "https://api.eu.mailgun.net/v3"

	return &Client{
		apiKey:     secrets.MailgunAPIKey,
		domain:     secrets.MailgunDomain,
		fromEmail:  secrets.MailgunFromEmail,
		fromName:   secrets.MailgunFromName,
		apiBaseURL: apiBase,
	}
}

// Mail represents an email to send.
type Mail struct {
	FromName  string
	FromEmail string
	ToName    string
	ToEmail   string
	Subject   string
	HTML      string
	Text      string
}

// Send sends an email using Mailgun HTTP API.
func (c *Client) Send(ctx context.Context, m Mail) error {
	if c.apiKey == "" || c.domain == "" {
		return errors.New("missing Mailgun API key or domain")
	}

	// Use default From if not provided
	fromEmail := m.FromEmail
	fromName := m.FromName
	if fromEmail == "" {
		fromEmail = c.fromEmail
	}
	if fromName == "" {
		fromName = c.fromName
	}

	fmt.Printf("📧 Sending email to %s with subject: %s\n", m.ToEmail, m.Subject)

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add form fields
	writer.WriteField("from", fmt.Sprintf("%s <%s>", fromName, fromEmail))
	writer.WriteField("to", fmt.Sprintf("%s <%s>", m.ToName, m.ToEmail))
	writer.WriteField("subject", m.Subject)

	if m.HTML != "" {
		writer.WriteField("html", m.HTML)
	}
	if m.Text != "" {
		writer.WriteField("text", m.Text)
	}

	contentType := writer.FormDataContentType()
	writer.Close()

	// Create HTTP request
	// Correct URL format: https://api.eu.mailgun.net/v3/{domain}/messages
	url := fmt.Sprintf("%s/%s/messages", c.apiBaseURL, c.domain)
	fmt.Printf("🔗 Mailgun API URL: %s\n", url)

	req, err := http.NewRequestWithContext(ctx, "POST", url, &buf)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.SetBasicAuth("api", c.apiKey)
	req.Header.Set("Content-Type", contentType)

	// Send request
	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, _ := io.ReadAll(resp.Body)

	// Check response
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("❌ Mailgun error: status=%d, body=%s\n", resp.StatusCode, string(body))
		return fmt.Errorf("mailgun error: status=%d, body=%s", resp.StatusCode, string(body))
	}

	fmt.Printf("✅ Email sent successfully to %s\n", m.ToEmail)
	fmt.Printf("📬 Mailgun Response: %s\n", string(body))
	fmt.Printf("⚠️  If using Sandbox: Make sure %s is authorized at https://app.mailgun.com/mg/sending/domains\n", m.ToEmail)
	return nil
}
