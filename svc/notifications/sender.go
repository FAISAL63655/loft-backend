package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"encore.app/pkg/mailer"
	"encore.app/pkg/templates"
	"encore.app/pkg/errs"
	"encore.dev/cron"
)

// background email processor (Queue + Retry up to 3 with backoff handled in DB trigger)

// ProcessEmailQueueResponse is the named response type for the private API
type ProcessEmailQueueResponse struct {
    Processed int `json:"processed"`
}

//encore:api private
func ProcessEmailQueue(ctx context.Context) (*ProcessEmailQueueResponse, error) {
	client := mailer.NewClient()
	// fetch a batch of queued email notifications ready to send
	rows, err := db.Stdlib().QueryContext(ctx, `
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
		res, err := db.Stdlib().ExecContext(ctx, `
			UPDATE notifications 
			SET status='sending'
			WHERE id=$1 AND (
				status='queued' OR (status='failed' AND next_retry_at IS NOT NULL AND next_retry_at <= NOW())
			)
		`, id)
		if err != nil {
			continue
		}
		ra, _ := res.RowsAffected()
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
			_, _ = db.Stdlib().ExecContext(ctx, `
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
			FromEmail: "noreply@loft-dughairi.com",
			ToName:    toName,
			ToEmail:   toEmail,
			Subject:   subject,
			HTML:      htmlBody,
			Text:      textBody,
		}
		err = client.Send(ctx, mail)
		if err == nil {
			_, _ = db.Stdlib().ExecContext(ctx, `UPDATE notifications SET status='sent', sent_at=NOW() WHERE id=$1`, id)
			processed++
			continue
		}
		// failure: increment retry_count and set failed_reason; trigger will schedule next_retry_at
		_, _ = db.Stdlib().ExecContext(ctx, `
			UPDATE notifications
			SET 
			  retry_count = retry_count + 1,
			  status = CASE WHEN retry_count + 1 >= max_retries THEN 'archived' ELSE 'failed' END,
			  failed_reason = $2
			WHERE id=$1`, id, err.Error())
	}
	return &ProcessEmailQueueResponse{Processed: processed}, nil
}

var _ = cron.NewJob("notifications-email-queue", cron.JobConfig{
	Title:    "Process email notifications queue",
	Every:    2 * cron.Minute,
	Endpoint: ProcessEmailQueue,
})

// Utility to enqueue an email notification
func EnqueueEmail(ctx context.Context, userID int64, templateID string, payload any) (int64, error) {
	buf, _ := json.Marshal(payload)
	var id int64
	if err := db.Stdlib().QueryRowContext(ctx, `
		INSERT INTO notifications (user_id, channel, template_id, payload, status)
		VALUES ($1,'email',$2,$3,'queued')
		RETURNING id
	`, userID, templateID, json.RawMessage(buf)).Scan(&id); err != nil {
		return 0, errs.New(errs.NotifQueueInsertFailed, "فشل إدراج الإشعار")
	}
	return id, nil
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
	if err := db.Stdlib().QueryRowContext(ctx, `
		INSERT INTO notifications (user_id, channel, template_id, payload, status)
		VALUES ($1,'internal',$2,$3,'queued')
		RETURNING id
	`, userID, templateID, json.RawMessage(buf)).Scan(&id); err != nil {
		return 0, errs.New(errs.NotifQueueInsertFailed, "فشل إدراج الإشعار الداخلي")
	}
	return id, nil
}
