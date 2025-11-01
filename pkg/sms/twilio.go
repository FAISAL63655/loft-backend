// Package sms provides SMS sending functionality via Twilio
package sms

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"encore.app/pkg/logger"
)

// TwilioConfig holds Twilio configuration
type TwilioConfig struct {
	AccountSID      string
	AuthToken       string
	FromNumber      string // Legacy: for direct SMS (optional)
	VerifyServiceID string // Twilio Verify Service SID (recommended)
	DevMode         bool   // If true, skip actual SMS sending and log OTP instead
}

// TwilioClient represents a Twilio SMS client
type TwilioClient struct {
	config     TwilioConfig
	httpClient *http.Client
}

// NewTwilioClient creates a new Twilio client
func NewTwilioClient(config TwilioConfig) *TwilioClient {
	return &TwilioClient{
		config: config,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SendSMS sends an SMS message via Twilio
func (c *TwilioClient) SendSMS(ctx context.Context, to, message string) error {
	// Dev mode: just log the OTP instead of sending
	if c.config.DevMode {
		logger.Info(ctx, "📱 [DEV MODE] SMS", logger.Fields{"to": to, "message": message})
		return nil
	}

	// Production mode: send via Twilio
	apiURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", c.config.AccountSID)

	data := url.Values{}
	data.Set("To", to)
	data.Set("From", c.config.FromNumber)
	data.Set("Body", message)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.config.AccountSID, c.config.AuthToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send SMS: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		var twilioError struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(body, &twilioError); err == nil {
			return fmt.Errorf("twilio error %d: %s", twilioError.Code, twilioError.Message)
		}
		return fmt.Errorf("twilio request failed with status %d: %s", resp.StatusCode, string(body))
	}

	logger.Info(ctx, "✅ SMS sent successfully", logger.Fields{"to": to})
	return nil
}

// SendOTP sends an OTP code via SMS
// If VerifyServiceID is configured, uses Twilio Verify API (recommended)
// Otherwise falls back to direct SMS sending
func (c *TwilioClient) SendOTP(ctx context.Context, phone, code string) error {
	logger.Info(ctx, "📱 SendOTP called", logger.Fields{
		"phone":              phone,
		"dev_mode":           c.config.DevMode,
		"has_verify_service": c.config.VerifyServiceID != "",
	})

	// Dev mode: just log the OTP
	if c.config.DevMode {
		logger.Info(ctx, "📱 [DEV MODE] OTP - Not sending actual SMS", logger.Fields{"to": phone, "code": code})
		return nil
	}

	// Use Twilio Verify if configured
	if c.config.VerifyServiceID != "" {
		logger.Info(ctx, "📱 Using Twilio Verify", logger.Fields{"phone": phone, "service_id": c.config.VerifyServiceID})
		return c.sendVerifyOTP(ctx, phone, code)
	}

	// Fallback to direct SMS
	logger.Info(ctx, "📱 Using direct SMS (fallback)", logger.Fields{"phone": phone, "from": c.config.FromNumber})
	message := fmt.Sprintf("رمز التحقق الخاص بك في لوفت الدغيري: %s\nصالح لمدة 10 دقائق", code)
	return c.SendSMS(ctx, phone, message)
}

// sendVerifyOTP sends OTP using Twilio Verify API
// Note: Twilio Verify generates its own OTP, so the 'code' parameter is ignored
func (c *TwilioClient) sendVerifyOTP(ctx context.Context, phone, _ string) error {
	apiURL := fmt.Sprintf("https://verify.twilio.com/v2/Services/%s/Verifications", c.config.VerifyServiceID)

	data := url.Values{}
	data.Set("To", phone)
	data.Set("Channel", "sms")
	data.Set("Locale", "ar") // Arabic locale

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create verify request: %w", err)
	}

	req.SetBasicAuth(c.config.AccountSID, c.config.AuthToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send verify OTP: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		var twilioError struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(body, &twilioError); err == nil {
			return fmt.Errorf("twilio verify error %d: %s", twilioError.Code, twilioError.Message)
		}
		return fmt.Errorf("twilio verify request failed with status %d: %s", resp.StatusCode, string(body))
	}

	logger.Info(ctx, "✅ Verify OTP sent successfully", logger.Fields{"to": phone})
	return nil
}

// VerifyOTP verifies an OTP code using Twilio Verify API
func (c *TwilioClient) VerifyOTP(ctx context.Context, phone, code string) error {
	// Dev mode: accept any code
	if c.config.DevMode {
		logger.Info(ctx, "✅ [DEV MODE] OTP verified", logger.Fields{"to": phone, "code": code})
		return nil
	}

	// Verify service must be configured
	if c.config.VerifyServiceID == "" {
		return fmt.Errorf("verify service not configured")
	}

	apiURL := fmt.Sprintf("https://verify.twilio.com/v2/Services/%s/VerificationCheck", c.config.VerifyServiceID)

	data := url.Values{}
	data.Set("To", phone)
	data.Set("Code", code)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create verify check request: %w", err)
	}

	req.SetBasicAuth(c.config.AccountSID, c.config.AuthToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to verify OTP: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var verifyResp struct {
		Status string `json:"status"`
		Valid  bool   `json:"valid"`
	}

	if err := json.Unmarshal(body, &verifyResp); err != nil {
		return fmt.Errorf("failed to parse verify response: %w", err)
	}

	if !verifyResp.Valid || verifyResp.Status != "approved" {
		return fmt.Errorf("invalid verification code")
	}

	logger.Info(ctx, "✅ OTP verified successfully", logger.Fields{"to": phone})
	return nil
}

// IsDevMode returns whether the client is in dev mode
func (c *TwilioClient) IsDevMode() bool {
	return c.config.DevMode
}

// HasVerifyService returns whether Twilio Verify Service is configured
func (c *TwilioClient) HasVerifyService() bool {
	return c.config.VerifyServiceID != ""
}
