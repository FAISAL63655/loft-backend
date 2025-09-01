package moyasar

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"encore.app/pkg/config"
	"encore.dev"
)

var secrets struct {
	MoyasarAPIKey        string //encore:secret
	MoyasarWebhookSecret string //encore:secret
}

func inTestMode() bool {
	cfg := config.GetGlobalManager()
	if cfg != nil {
		settings := cfg.GetSettings()
		if settings != nil && settings.PaymentsTestMode {
			return true
		}
	}
	// Fallback to local env
	return encore.Meta().Environment.Type == encore.EnvLocal
}

// VerifySignature verifies Moyasar webhook signature (HMAC-SHA256 on raw body)
func VerifySignature(rawBody []byte, signatureHeader string) bool {
	if secrets.MoyasarWebhookSecret == "" {
		// Allow in test/local mode
		if inTestMode() {
			return true
		}
		return false
	}
	if signatureHeader == "" {
		return false
	}
	expected := computeHMAC(rawBody, secrets.MoyasarWebhookSecret)
	
	// Parse signature header safely - handle common formats
	provided := ""
	if strings.HasPrefix(signatureHeader, "sha256=") {
		// Format: sha256=<64-char-hex>
		if len(signatureHeader) == 71 { // "sha256=" (7) + 64 hex chars
			provided = signatureHeader[7:]
		}
	} else if strings.Contains(signatureHeader, "t=") && strings.Contains(signatureHeader, ",v1=") {
		// Stripe-style format: t=timestamp,v1=signature
		parts := strings.Split(signatureHeader, ",")
		for _, part := range parts {
			if strings.HasPrefix(part, "v1=") && len(part) == 67 { // "v1=" (3) + 64 hex chars
				provided = part[3:]
				break
			}
		}
	} else if len(signatureHeader) == 64 {
		// Raw 64-character hex signature
		provided = signatureHeader
	}
	
	// Validate hex format
	if provided == "" || len(provided) != 64 {
		return false
	}
	for _, char := range provided {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}
	
	return hmac.Equal([]byte(strings.ToLower(provided)), []byte(expected))
}

func computeHMAC(body []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// CreateInvoice creates a Moyasar invoice/payment link and returns (gatewayRef, sessionURL)
type createInvoiceReq struct {
	Amount      int               `json:"amount"`
	Currency    string            `json:"currency"`
	Description string            `json:"description,omitempty"`
	CallbackURL string            `json:"callback_url,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type createInvoiceResp struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	URL    string `json:"url"`
}

func CreateInvoice(amountHalalas int, currency, description, callbackURL string, metadata map[string]string) (string, string, error) {
	if secrets.MoyasarAPIKey == "" {
		if inTestMode() {
			id := fmt.Sprintf("test_%d", time.Now().UTC().UnixNano())
			url := "https://sandbox.moyasar.com/pay/" + id
			return id, url, nil
		}
		return "", "", fmt.Errorf("moyasar api key not set")
	}
	payload := &createInvoiceReq{Amount: amountHalalas, Currency: currency, Description: description, CallbackURL: callbackURL, Metadata: metadata}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, "https://api.moyasar.com/v1/invoices", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(secrets.MoyasarAPIKey, "")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("moyasar create invoice failed: %s", resp.Status)
	}
	var out createInvoiceResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", err
	}
	if out.ID == "" || out.URL == "" {
		return "", "", fmt.Errorf("moyasar response missing fields")
	}
	return out.ID, out.URL, nil
}

// RefundPayment requests a refund (partial or full) for a payment by gateway ref
func RefundPayment(gatewayRef string, amountHalalas int) error {
	if secrets.MoyasarAPIKey == "" {
		if inTestMode() {
			return nil
		}
		return fmt.Errorf("moyasar api key not set")
	}
	payload := map[string]any{
		"amount": amountHalalas,
	}
	b, _ := json.Marshal(payload)
	url := fmt.Sprintf("https://api.moyasar.com/v1/payments/%s/refund", gatewayRef)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(secrets.MoyasarAPIKey, "")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("moyasar refund failed: %s", resp.Status)
	}
	return nil
}
