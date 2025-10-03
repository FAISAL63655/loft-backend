package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"encore.dev"
	"encore.dev/cron"
	"encore.dev/storage/sqldb"

	"encore.app/pkg/errs"
	"encore.app/pkg/mailer"
	"encore.app/pkg/templates"
)

// Ensure db is available
var senderDB = sqldb.Named("coredb")

// background email processor (Queue + Retry up to 3 with backoff handled in DB trigger)

// ProcessEmailQueueResponse is the named response type for the private API
type ProcessEmailQueueResponse struct {
	Processed int `json:"processed"`
}

//encore:api private
func ProcessEmailQueue(ctx context.Context) (*ProcessEmailQueueResponse, error) {
	client := mailer.NewClient()
	// fetch a batch of queued email notifications ready to send
	rows, err := senderDB.Query(ctx, `
		SELECT id, user_id, template_id, payload
		FROM notifications
		WHERE channel='email'
		  AND (
			status = 'queued'
			OR (
			  status = 'failed' AND next_retry_at IS NOT NULL AND next_retry_at <= NOW()
			)
		  )
		ORDER BY created_at ASC
		LIMIT 50`)
	if err != nil {
		return nil, errs.New(errs.NotifQueueQueryFailed, "فشل الاستعلام عن الطابور")
	}
	defer rows.Close()
	processed := 0
	for rows.Next() {
		var id int64
		var userID int64
		var templateID string
		var payload json.RawMessage
		if err := rows.Scan(&id, &userID, &templateID, &payload); err != nil {
			return nil, errs.New(errs.NotifQueueQueryFailed, "فشل القراءة")
		}
		// mark as sending (claim)
		res, err := senderDB.Exec(ctx, `
			UPDATE notifications 
			SET status='sending'
			WHERE id=$1 AND (
				status='queued' OR (status='failed' AND next_retry_at IS NOT NULL AND next_retry_at <= NOW())
			)
		`, id)
		if err != nil {
			continue
		}
		ra := res.RowsAffected()
		if ra == 0 {
			// someone else claimed it
			continue
		}
		// build email from template
		var pl map[string]any
		_ = json.Unmarshal(payload, &pl)

		// Normalize template keys: ensure capitalized keys used by templates
		if nameLower, ok := pl["name"].(string); ok {
			if _, exists := pl["Name"]; !exists {
				pl["Name"] = nameLower
			}
		}

		// Get user's preferred language (default to Arabic)
		lang := "ar"
		if userLang, ok := pl["language"].(string); ok {
			lang = userLang
		}

		// Extract recipient safely
		toEmail, _ := pl["email"].(string)
		toName, _ := pl["name"].(string)
		if toName == "" {
			toName = "Customer"
		}
		if toEmail == "" {
			// fail fast: missing recipient email
			_, _ = senderDB.Exec(ctx, `
				UPDATE notifications
				SET 
				  retry_count = retry_count + 1,
				  status = CASE WHEN retry_count + 1 >= max_retries THEN 'archived' ELSE 'failed' END,
				  failed_reason = $2
				WHERE id = $1
			`, id, "missing recipient email")
			continue
		}

		// Render template
		subject, htmlBody, textBody, err := templates.RenderTemplate(templateID, lang, templates.TemplateData(pl))
		if err != nil {
			// fallback to simple notification if template fails
			subject = templateID
			htmlBody = "<p>Notification</p>"
			textBody = "Notification"
		}

		mail := mailer.Mail{
			FromName:  "Loft Dughairi",
			FromEmail: "contact@dughairiloft.com",
			ToName:    toName,
			ToEmail:   toEmail,
			Subject:   subject,
			HTML:      htmlBody,
			Text:      textBody,
		}
		err = client.Send(ctx, mail)
		if err == nil {
			result, updateErr := senderDB.Exec(ctx, `UPDATE notifications SET status='sent', sent_at=NOW() WHERE id=$1`, id)
			if updateErr != nil {
				// Log update error but don't fail the whole process
				fmt.Printf("ERROR: Failed to update notification %d to sent: %v\n", id, updateErr)
			} else if result.RowsAffected() == 0 {
				fmt.Printf("WARNING: No rows updated for notification %d\n", id)
			} else {
				processed++
			}
			continue
		}
		// Log send error for debugging
		fmt.Printf("ERROR: Failed to send email for notification %d: %v\n", id, err)
		// failure: increment retry_count and set failed_reason; trigger will schedule next_retry_at
		result, updateErr := senderDB.Exec(ctx, `
			UPDATE notifications
			SET 
			  retry_count = retry_count + 1,
			  status = CASE WHEN retry_count + 1 >= max_retries THEN 'archived'::notification_status ELSE 'failed'::notification_status END,
			  failed_reason = $2
			WHERE id=$1`, id, err.Error())
		if updateErr != nil {
			fmt.Printf("ERROR: Failed to update notification %d to failed: %v\n", id, updateErr)
		} else if result.RowsAffected() == 0 {
			fmt.Printf("WARNING: No rows updated for failed notification %d\n", id)
		}
	}
	return &ProcessEmailQueueResponse{Processed: processed}, nil
}

var _ = cron.NewJob("notifications-email-queue", cron.JobConfig{
	Title:    "Process email notifications queue",
	Every:    cron.Minute,
	Endpoint: ProcessEmailQueue,
})

// Utility to enqueue an email notification
func EnqueueEmail(ctx context.Context, userID int64, templateID string, payload any) (int64, error) {
	// Try to extract verification_code if present for idempotency
	var verificationCode string
	switch pl := payload.(type) {
	case map[string]any:
		if v, ok := pl["verification_code"].(string); ok {
			verificationCode = v
		}
	case map[string]string:
		if v, ok := pl["verification_code"]; ok {
			verificationCode = v
		}
	}

	// Deduplicate only for verification-like templates with a code
	if verificationCode != "" && (templateID == "email_verification" || templateID == "password_reset") {
		var existingID int64
		err := senderDB.QueryRow(ctx, `
			SELECT id FROM notifications
			WHERE user_id = $1 AND channel = 'email' AND template_id = $2
			  AND status IN ('queued','sending')
			  AND payload->>'verification_code' = $3
			ORDER BY created_at DESC
			LIMIT 1
		`, userID, templateID, verificationCode).Scan(&existingID)
		if err == nil && existingID > 0 {
			return existingID, nil
		}
	}

	buf, _ := json.Marshal(payload)
	var id int64
	if err := senderDB.QueryRow(ctx, `
		INSERT INTO notifications (user_id, channel, template_id, payload, status)
		VALUES ($1,'email',$2,$3,'queued')
		RETURNING id
	`, userID, templateID, json.RawMessage(buf)).Scan(&id); err != nil {
		// Add DB identity diagnostics to help identify missing GRANTs
		dbUser, dbSchema := "", ""
		_ = senderDB.QueryRow(ctx, "SELECT current_user, current_schema();").Scan(&dbUser, &dbSchema)
		details := map[string]any{"cause": err.Error()}
		if dbUser != "" {
			details["db_user"] = dbUser
		}
		if dbSchema != "" {
			details["db_schema"] = dbSchema
		}
		return 0, errs.EDetails(ctx, errs.NotifQueueInsertFailed, "فشل إدراج الإشعار", details)
	}

	// Immediately process in local dev (cron does not run locally)
	if encore.Meta().Environment.Type == encore.EnvDevelopment && encore.Meta().Environment.Cloud == encore.CloudLocal {
		go func() {
			c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, _ = ProcessEmailQueue(c)
		}()
		return id, nil
	}

	// For cloud and other environments: trigger immediate processing for critical templates
	if isImmediateTemplate(templateID) {
		go func() {
			c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = ProcessEmailQueue(c)
		}()
	}
	return id, nil
}

// isImmediateTemplate marks templates that should be sent with minimal delay
func isImmediateTemplate(tpl string) bool {
	switch tpl {
	case "email_verification", "password_reset":
		return true
	default:
		return false
	}
}

// getNextRetryAt replicates DB-side backoff logic (optional helper)
func getNextRetryAt(retryCount, maxRetries int) (sql.NullTime, error) {
	if retryCount >= maxRetries {
		return sql.NullTime{}, errors.New("max retries reached")
	}
	minutes := time.Duration(1<<retryCount) * 5
	return sql.NullTime{Time: time.Now().Add(minutes * time.Minute), Valid: true}, nil
}

// Utility to enqueue an internal (inbox) notification
func EnqueueInternal(ctx context.Context, userID int64, templateID string, payload any) (int64, error) {
	buf, _ := json.Marshal(payload)
	var id int64
	if err := senderDB.QueryRow(ctx, `
		INSERT INTO notifications (user_id, channel, template_id, payload, status)
		VALUES ($1,'internal',$2,$3,'queued')
		RETURNING id
	`, userID, templateID, json.RawMessage(buf)).Scan(&id); err != nil {
		// Add DB identity diagnostics to help identify missing GRANTs
		dbUser, dbSchema := "", ""
		_ = senderDB.QueryRow(ctx, "SELECT current_user, current_schema();").Scan(&dbUser, &dbSchema)
		details := map[string]any{"cause": err.Error()}
		if dbUser != "" {
			details["db_user"] = dbUser
		}
		if dbSchema != "" {
			details["db_schema"] = dbSchema
		}
		return 0, errs.EDetails(ctx, errs.NotifQueueInsertFailed, "فشل إدراج الإشعار الداخلي", details)
	}
	return id, nil
}
