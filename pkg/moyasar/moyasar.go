package moyasar

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"encore.app/pkg/config"
)

var secrets struct {
	MoyasarAPIKey        string //encore:secret
	MoyasarWebhookSecret string //encore:secret
}

func inTestMode() bool {
    // Test mode strictly controlled via system settings
    if s := config.GetSettings(); s != nil && s.PaymentsTestMode {
        return true
    }
    return false
}

func VerifySignature(rawBody []byte, signatureHeader string) bool {
    // Allow skipping verification only when PaymentsTestMode is enabled.
    if s := config.GetSettings(); s != nil && s.PaymentsTestMode {
        return true
    }
    if secrets.MoyasarWebhookSecret == "" || strings.TrimSpace(signatureHeader) == "" {
        return false
    }

    // Extract candidate signature values from header
    // Supported formats: "sha256=<sig>", "t=..., v1=<sig>", raw value, comma-separated list
    candidates := []string{}
    timestamp := ""
    hdr := strings.TrimSpace(signatureHeader)
    if strings.HasPrefix(hdr, "sha256=") {
        candidates = append(candidates, strings.TrimSpace(hdr[7:]))
    }
    for _, part := range strings.Split(hdr, ",") {
        kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
        if len(kv) == 2 {
            if strings.EqualFold(kv[0], "t") {
                timestamp = strings.TrimSpace(kv[1])
            } else {
                candidates = append(candidates, strings.TrimSpace(kv[1]))
            }
        }
    }
    // Also consider the raw header value
    candidates = append(candidates, hdr)

    // Build secret key variants: raw, base64-decoded, hex-decoded
    trimmedSecret := strings.TrimSpace(secrets.MoyasarWebhookSecret)
    secretVariants := [][]byte{[]byte(trimmedSecret)}
    if dec, err := decodeBase64(trimmedSecret); err == nil && len(dec) > 0 {
        secretVariants = append(secretVariants, dec)
    }
    if hexKey, err := hex.DecodeString(trimmedSecret); err == nil && len(hexKey) > 0 {
        secretVariants = append(secretVariants, hexKey)
    }

    // Build message variants: raw body and timestamped variant if timestamp present
    messages := [][]byte{rawBody}
    if timestamp != "" {
        messages = append(messages, []byte(timestamp+"."+string(rawBody)))
    }

    // Precompute expected signatures for all variants
    type exp struct{ hex string; raw []byte }
    expected := make([]exp, 0, len(secretVariants)*len(messages))
    for _, key := range secretVariants {
        for _, msg := range messages {
            e := computeHMACWithKey(msg, key)
            expected = append(expected, exp{hex: hex.EncodeToString(e), raw: e})
        }
    }

    // Try to match any candidate either as 64-hex or as base64 of raw HMAC
    for _, c := range candidates {
        c = strings.Trim(c, " \t\r\n")
        if c == "" {
            continue
        }
        // Hex path
        if len(c) == 64 {
            hexOK := true
            for _, ch := range c {
                if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
                    hexOK = false
                    break
                }
            }
            if hexOK {
                cLower := strings.ToLower(c)
                for _, e := range expected {
                    if hmac.Equal([]byte(cLower), []byte(e.hex)) {
                        return true
                    }
                }
            }
        }
        // Base64 path
        if dec, err := decodeBase64(c); err == nil {
            for _, e := range expected {
                if hmac.Equal(dec, e.raw) {
                    return true
                }
            }
        }
    }
    return false
}

// VerifySecretToken validates a plain secret token carried in the webhook payload (non-HMAC).
// Some providers (including Moyasar) include `secret_token` in the webhook object instead of
// an HMAC signature header. This helper compares the provided token with the configured secret.
func VerifySecretToken(token string) bool {
    // Allow skipping verification only when PaymentsTestMode is enabled.
    if s := config.GetSettings(); s != nil && s.PaymentsTestMode {
        return true
    }
    t := strings.TrimSpace(token)
    s := strings.TrimSpace(secrets.MoyasarWebhookSecret)
    if t == "" || s == "" {
        return false
    }
    return t == s
}

func computeHMAC(body []byte, secret string) string {
    h := hmac.New(sha256.New, []byte(secret))
    h.Write(body)
    return hex.EncodeToString(h.Sum(nil))
}

func computeHMACBytes(body []byte, secret string) []byte {
    h := hmac.New(sha256.New, []byte(secret))
    h.Reset()
    h.Write(body)
    return h.Sum(nil)
}

func computeHMACWithKey(body []byte, key []byte) []byte {
  h := hmac.New(sha256.New, key)
  h.Write(body)
  return h.Sum(nil)
}

func decodeBase64(s string) ([]byte, error) {
  // Try standard and URL-compatible base64 without/with padding
  if b, err := base64.StdEncoding.DecodeString(s); err == nil {
    return b, nil
  }
  if b, err := base64.URLEncoding.DecodeString(s); err == nil {
    return b, nil
  }
  // Add padding if missing
  if m := len(s) % 4; m != 0 {
    s2 := s + strings.Repeat("=", 4-m)
    if b, err := base64.StdEncoding.DecodeString(s2); err == nil {
      return b, nil
    }
    if b, err := base64.URLEncoding.DecodeString(s2); err == nil {
      return b, nil
    }
  }
  return nil, fmt.Errorf("invalid base64")
}

// CreateInvoice creates a moyasar invoice/payment link and returns (gatewayRef, sessionURL)
// successURL: browser redirect after successful payment (user-facing)
// backURL: browser redirect when user clicks back (user-facing)
// callbackURL: server-to-server notification endpoint for invoice paid (optional)
type createInvoiceReq struct {
    Amount      int               `json:"amount"`
    Currency    string            `json:"currency"`
    Description string            `json:"description,omitempty"`
    SuccessURL  string            `json:"success_url,omitempty"`
    BackURL     string            `json:"back_url,omitempty"`
    CallbackURL string            `json:"callback_url,omitempty"`
    Metadata    map[string]string `json:"metadata,omitempty"`
}

type createInvoiceResp struct {
    ID     string `json:"id"`
    Status string `json:"status"`
    URL    string `json:"url"`
}

func CreateInvoice(amountHalalas int, currency, description, successURL, backURL, callbackURL string, metadata map[string]string) (string, string, error) {
    // In test/local mode without API key, return a stub id and redirect directly back to our app
    if secrets.MoyasarAPIKey == "" {
        if inTestMode() {
            id := fmt.Sprintf("test_%d", time.Now().UTC().UnixNano())
            // Return to our app's pending page so sessionStorage fallback can resolve payment_id
            return id, successURL, nil
        }
        return "", "", fmt.Errorf("moyasar api key not set")
    }
    apiKey := strings.TrimSpace(secrets.MoyasarAPIKey)
    if apiKey == "" {
        return "", "", fmt.Errorf("moyasar api key not set (empty after trim)")
    }
    payload := &createInvoiceReq{
        Amount:      amountHalalas,
        Currency:    currency,
        Description: description,
        SuccessURL:  successURL,
        BackURL:     backURL,
        CallbackURL: callbackURL,
        Metadata:    metadata,
    }
    body, _ := json.Marshal(payload)
    req, err := http.NewRequest(http.MethodPost, "https://api.moyasar.com/v1/invoices", bytes.NewReader(body))
    if err != nil {
        return "", "", err
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Accept", "application/json")
    req.SetBasicAuth(apiKey, "")
    client := &http.Client{Timeout: 15 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return "", "", err
    }
    defer resp.Body.Close()
    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        // Read response body for diagnostics (no secrets included)
        respBody, _ := io.ReadAll(resp.Body)
        // Truncate body to avoid excessive logs
        if len(respBody) > 512 {
            respBody = respBody[:512]
        }
        return "", "", fmt.Errorf("moyasar create invoice failed: %s: %s", resp.Status, string(respBody))
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
    // In test/local mode, short-circuit and succeed regardless of API key presence
    if inTestMode() {
        return nil
    }
    if secrets.MoyasarAPIKey == "" {
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
