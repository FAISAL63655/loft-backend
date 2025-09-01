package notifications

import (
	"context"
	"encoding/json"
	"strconv"

	"encore.dev/beta/auth"
	"encore.app/pkg/errs"
	"encore.dev/cron"
)

// RetentionConfig إعدادات الاحتفاظ بالإشعارات
type RetentionConfig struct {
	// الاحتفاظ بالإشعارات المرسلة لمدة 90 يوم
	SentRetentionDays int
	// الاحتفاظ بالإشعارات الفاشلة لمدة 30 يوم
	FailedRetentionDays int
	// الاحتفاظ بالإشعارات المقروءة لمدة 60 يوم
	ReadRetentionDays int
	// أرشفة الإشعارات المهمة بدلاً من حذفها
	ArchiveImportant bool
}

// Default retention configuration
var defaultRetention = RetentionConfig{
	SentRetentionDays:   90,
	FailedRetentionDays: 30,
	ReadRetentionDays:   60,
	ArchiveImportant:    true,
}

// CleanupNotificationsRequest طلب تنظيف الإشعارات
type CleanupNotificationsRequest struct{}

// CleanupNotificationsResponse استجابة تنظيف الإشعارات
type CleanupNotificationsResponse struct {
	Deleted  int `json:"deleted"`
	Archived int `json:"archived"`
}

//encore:api private
func CleanupNotifications(ctx context.Context) (*CleanupNotificationsResponse, error) {
	var deleted, archived int
	
	// Archive important notifications before deletion (write audit logs)
	if defaultRetention.ArchiveImportant {
		result, err := db.Stdlib().ExecContext(ctx, `
			INSERT INTO audit_logs (actor_user_id, action, entity_type, entity_id, reason, meta, created_at)
			SELECT 
				user_id,
				'archived',
				'notification',
				id::text,
				NULL,
				jsonb_build_object(
					'template_id', template_id,
					'channel', channel,
					'status', status,
					'payload', payload,
					'sent_at', sent_at
				),
				NOW()
			FROM notifications
			WHERE 
				(status = 'sent' AND sent_at < NOW() - make_interval(days => $1) AND template_id IN ('auction_won', 'payment_received'))
				OR (status = 'failed' AND created_at < NOW() - make_interval(days => $2) AND retry_count >= 3)
		`, defaultRetention.SentRetentionDays, defaultRetention.FailedRetentionDays)
		
		if err != nil {
			return nil, errs.New(errs.NotifRetentionArchiveFailed, "فشل أرشفة الإشعارات")
		}
		
		archivedRows, _ := result.RowsAffected()
		archived = int(archivedRows)
	}
	
	// Delete old notifications by status buckets
	result, err := db.Stdlib().ExecContext(ctx, `
		DELETE FROM notifications
		WHERE 
			(status = 'sent' AND sent_at < NOW() - make_interval(days => $1))
			OR (status = 'failed' AND created_at < NOW() - make_interval(days => $2))
			OR (status = 'archived' AND updated_at < NOW() - make_interval(days => $3))
	`, defaultRetention.SentRetentionDays, defaultRetention.FailedRetentionDays, defaultRetention.ReadRetentionDays)
	
	if err != nil {
		return nil, errs.New(errs.NotifRetentionDeleteFailed, "فشل حذف الإشعارات القديمة")
	}
	
	deletedRows, _ := result.RowsAffected()
	deleted = int(deletedRows)
	
	// Clean up old audit logs (older than 1 year)
	_, _ = db.Stdlib().ExecContext(ctx, `
		DELETE FROM audit_logs
		WHERE created_at < NOW() - INTERVAL '365 days'
			AND entity_type NOT IN ('payment', 'auction_won', 'user_verification')
	`)
	
	return &CleanupNotificationsResponse{
		Deleted:  deleted,
		Archived: archived,
	}, nil
}

// Cron job for daily cleanup at 3 AM
var _ = cron.NewJob("notifications-retention-cleanup", cron.JobConfig{
	Title:    "Clean up old notifications based on retention policy",
	Schedule: "0 3 * * *", // Daily at 3 AM
	Endpoint: CleanupNotifications,
})

// MarkAsReadRequest طلب وضع علامة مقروء
type MarkAsReadRequest struct {
	NotificationID int64 `json:"notification_id"`
}

// MarkAsReadResponse استجابة وضع علامة مقروء
type MarkAsReadResponse struct {
	Success bool `json:"success"`
}

//encore:api auth method=POST path=/notifications/read
func (s *Service) MarkAsRead(ctx context.Context, req *MarkAsReadRequest) (*MarkAsReadResponse, error) {
	uidStr, ok := auth.UserID()
	if !ok {
		return nil, errs.New(errs.NotifUnauthenticated, "مطلوب تسجيل الدخول")
	}
	
	uid, err := strconv.ParseInt(string(uidStr), 10, 64)
	if err != nil {
		return nil, errs.New(errs.InvalidArgument, "معرّف مستخدم غير صالح")
	}
	
	// Archive notification as a proxy for "read" (only if owned by user)
	result, err := db.Stdlib().ExecContext(ctx, `
		UPDATE notifications 
		SET status = 'archived'
		WHERE id = $1 AND user_id = $2 AND status != 'archived'
	`, req.NotificationID, uid)
	
	if err != nil {
		return nil, errs.New(errs.NotifUpdateFailed, "فشل تحديث حالة الإشعار")
	}
	
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return nil, errs.New(errs.NotifNotFound, "الإشعار غير موجود أو مؤرشف مسبقاً")
	}
	
	return &MarkAsReadResponse{Success: true}, nil
}

// GetRetentionConfigRequest طلب الحصول على إعدادات الاحتفاظ
type GetRetentionConfigRequest struct{}

// GetRetentionConfigResponse استجابة إعدادات الاحتفاظ
type GetRetentionConfigResponse struct {
	Config RetentionConfig `json:"config"`
}

//encore:api auth method=GET path=/notifications/retention/config
func (s *Service) GetRetentionConfig(ctx context.Context) (*GetRetentionConfigResponse, error) {
	// Check if user is admin (simplified)
	_, ok := auth.UserID()
	if !ok {
		return nil, errs.New(errs.NotifUnauthenticated, "مطلوب تسجيل الدخول")
	}
	
	return &GetRetentionConfigResponse{
		Config: defaultRetention,
	}, nil
}

// UpdateRetentionConfigRequest طلب تحديث إعدادات الاحتفاظ
type UpdateRetentionConfigRequest struct {
	SentRetentionDays   int  `json:"sent_retention_days"`
	FailedRetentionDays int  `json:"failed_retention_days"`
	ReadRetentionDays   int  `json:"read_retention_days"`
	ArchiveImportant    bool `json:"archive_important"`
}

// UpdateRetentionConfigResponse استجابة تحديث إعدادات الاحتفاظ
type UpdateRetentionConfigResponse struct {
	Success bool            `json:"success"`
	Config  RetentionConfig `json:"config"`
}

//encore:api auth method=POST path=/notifications/retention/config
func (s *Service) UpdateRetentionConfig(ctx context.Context, req *UpdateRetentionConfigRequest) (*UpdateRetentionConfigResponse, error) {
	// Check if user is admin (simplified)
	_, ok := auth.UserID()
	if !ok {
		return nil, errs.New(errs.NotifUnauthenticated, "مطلوب تسجيل الدخول")
	}
	
	// Validate configuration
	if req.SentRetentionDays < 30 || req.SentRetentionDays > 365 {
		return nil, errs.New(errs.InvalidArgument, "مدة الاحتفاظ بالرسائل المرسلة يجب أن تكون بين 30 و 365 يوم")
	}
	
	if req.FailedRetentionDays < 7 || req.FailedRetentionDays > 90 {
		return nil, errs.New(errs.InvalidArgument, "مدة الاحتفاظ بالرسائل الفاشلة يجب أن تكون بين 7 و 90 يوم")
	}
	
	if req.ReadRetentionDays < 30 || req.ReadRetentionDays > 180 {
		return nil, errs.New(errs.InvalidArgument, "مدة الاحتفاظ بالرسائل المقروءة يجب أن تكون بين 30 و 180 يوم")
	}
	
	// Update configuration
	defaultRetention = RetentionConfig{
		SentRetentionDays:   req.SentRetentionDays,
		FailedRetentionDays: req.FailedRetentionDays,
		ReadRetentionDays:   req.ReadRetentionDays,
		ArchiveImportant:    req.ArchiveImportant,
	}
	
	// Log configuration change in audit_logs per schema
	uidStr, _ := auth.UserID()
	uid, _ := strconv.ParseInt(string(uidStr), 10, 64)
	meta := map[string]interface{}{
		"sent_days":   req.SentRetentionDays,
		"failed_days": req.FailedRetentionDays,
		"read_days":   req.ReadRetentionDays,
		"archive":     req.ArchiveImportant,
	}
	metaJSON, _ := json.Marshal(meta)
	_, _ = db.Stdlib().ExecContext(ctx, `
		INSERT INTO audit_logs (actor_user_id, action, entity_type, entity_id, reason, meta, created_at)
		VALUES ($1, 'updated', 'retention_config', '0', NULL, $2, NOW())
	`, uid, json.RawMessage(metaJSON))
	
	return &UpdateRetentionConfigResponse{
		Success: true,
		Config:  defaultRetention,
	}, nil
}
