package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// secrets holds Encore-managed secrets for the mailer integration.
// The field name (SendGridAPIKey) is the secret key name in Encore Secrets.
var secrets struct {
	SendGridAPIKey string //encore:secret
}

// Client provides email sending via SendGrid's v3 HTTP API.
type Client struct {
	httpClient *http.Client
}

// NewClient constructs a new mailer client.
func NewClient() *Client {
	return &Client{httpClient: &http.Client{Timeout: 10 * time.Second}}
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

// Send sends an email using SendGrid.
func (c *Client) Send(ctx context.Context, m Mail) error {
	if secrets.SendGridAPIKey == "" {
		return errors.New("missing SendGridAPIKey secret")
	}
	body := map[string]any{
		"personalizations": []any{
			map[string]any{
				"to": []any{map[string]any{"email": m.ToEmail, "name": m.ToName}},
			},
		},
		"from":    map[string]any{"email": m.FromEmail, "name": m.FromName},
		"subject": m.Subject,
		"content": []any{
			map[string]any{"type": "text/plain", "value": m.Text},
			map[string]any{"type": "text/html", "value": m.HTML},
		},
	}
	buf, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.sendgrid.com/v3/mail/send", bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", secrets.SendGridAPIKey))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// SendGrid returns 202 on accepted
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("sendgrid error status: %d", resp.StatusCode)
	}
	return nil
}
