-- 0011_notifications_grants.up.sql
-- منح صلاحيات الكتابة/الحذف/التعديل على جدول الإشعارات وسجلات التدقيق
-- وتمكين الوصول إلى التسلسلات ذات الصلة لدور التطبيق (encore-read)

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'encore-read') THEN
        RAISE NOTICE 'Role "encore-read" not found; skipping grants.';
    ELSE
        -- الصلاحيات على جدول notifications
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.notifications TO "encore-read";
        -- التسلسل الخاص بالمفتاح الأساسي
        GRANT USAGE, SELECT ON SEQUENCE public.notifications_id_seq TO "encore-read";

        -- الصلاحيات على جدول audit_logs (مطلوبة للأرشفة والتدقيق)
        GRANT SELECT, INSERT ON TABLE public.audit_logs TO "encore-read";
        -- التسلسل الخاص بالمفتاح الأساسي
        GRANT USAGE, SELECT ON SEQUENCE public.audit_logs_id_seq TO "encore-read";
    END IF;
END
$$;

-- تهيئة الصلاحيات الافتراضية للمستقبل (اختياري لكن مستحسن)
-- لاحظ أن هذه تؤثر على الكائنات الجديدة التي ينشئها مالك المخطط لاحقاً
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO "encore-read";
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO "encore-read";
