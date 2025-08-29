-- seed_dev.sql
-- بيانات التطوير الأولية
-- منصة لوفت الدغيري للحمام الزاجل

-- إنشاء مستخدم إداري للتطوير (كلمة المرور: admin123 - يجب تغييرها)
-- hash للكلمة admin123 باستخدام Argon2id
INSERT INTO users (name, email, password_hash, role, state, email_verified_at, city_id) VALUES
('مدير النظام', 'admin@loft-dughairi.com', '$argon2id$v=19$m=65536,t=3,p=2$placeholder_salt$placeholder_hash', 'admin', 'active', NOW(), 1)
ON CONFLICT (email) DO NOTHING;

-- تعليق: كلمة المرور المؤقتة هي admin123 ويجب تغييرها فور أول تسجيل دخول
-- هذا الملف للتطوير فقط ولا يجب تشغيله في الإنتاج
