package templates

import (
	"bytes"
	"html/template"
	"sort"

	"encore.app/pkg/errs"
)

// EmailTemplate يمثل قالب بريد إلكتروني
type EmailTemplate struct {
	ID          string
	Subject     map[string]string // multi-language subjects
	HTMLBody    map[string]string // multi-language HTML templates
	TextBody    map[string]string // multi-language text templates
	Description string
}

// TemplateData يمثل البيانات المستخدمة في القوالب
type TemplateData map[string]interface{}

var templates = map[string]*EmailTemplate{
	"email_verification": {
		ID:          "email_verification",
		Description: "رمز تفعيل البريد الإلكتروني",
		Subject: map[string]string{
			"ar": "رمز التفعيل الخاص بك: {{.verification_code}}",
			"en": "Your Verification Code: {{.verification_code}}",
		},
		HTMLBody: map[string]string{
			"ar": `<!DOCTYPE html>
<html dir="rtl" lang="ar">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, 'Tajawal', sans-serif;
            line-height: 1.6;
            color: #333;
            direction: rtl;
            background: #f5f5f5;
        }
        .email-wrapper {
            max-width: 600px;
            margin: 20px auto;
            background: #ffffff;
            border-radius: 16px;
            overflow: hidden;
            box-shadow: 0 4px 20px rgba(0,0,0,0.08);
        }
        .header {
            background: linear-gradient(135deg, #4A9B8E 0%, #2E7D6E 100%);
            color: white;
            padding: 40px 30px;
            text-align: center;
        }
        .logo {
            font-size: 24px;
            font-weight: 800;
            margin-bottom: 10px;
        }
        .logo-subtitle {
            font-size: 13px;
            opacity: 0.9;
            font-weight: 400;
        }
        .header h1 {
            font-size: 22px;
            margin-top: 20px;
            font-weight: 700;
        }
        .content {
            padding: 32px 30px;
            background: white;
        }
        .greeting {
            font-size: 18px;
            color: #2E7D6E;
            margin-bottom: 16px;
            font-weight: 600;
        }
        .message {
            color: #555;
            margin-bottom: 24px;
            line-height: 1.8;
        }
        .code-container {
            text-align: center;
            margin: 24px 0;
            padding: 20px;
            background: linear-gradient(135deg, #f0f9f7 0%, #e6f4f1 100%);
            border-radius: 12px;
            border: 2px dashed #4A9B8E;
        }
        .code-label {
            font-size: 13px;
            color: #6B7B8C;
            margin-bottom: 8px;
            font-weight: 600;
            letter-spacing: .3px;
        }
        .code {
            font-size: 28px;
            font-weight: 800;
            letter-spacing: 6px;
            color: #2E7D6E;
            font-family: 'Courier New', monospace;
        }
        .notice {
            background: #fff9e6;
            border-right: 4px solid #ffc107;
            padding: 14px;
            margin: 20px 0;
            border-radius: 6px;
        }
        .notice p {
            color: #856404;
            font-size: 14px;
            margin: 0;
        }
        .footer {
            background: #f8f9fa;
            padding: 24px;
            text-align: center;
            border-top: 1px solid #e9ecef;
        }
        .footer-brand {
            color: #4A9B8E;
            font-weight: 700;
            font-size: 16px;
            margin-bottom: 6px;
        }
        .footer-text {
            color: #6c757d;
            font-size: 13px;
            line-height: 1.6;
        }
        .social-links { margin: 12px 0; }
        .social-links a {
            display: inline-block;
            margin: 0 8px;
            color: #6B7B8C;
            text-decoration: none;
            font-size: 13px;
        }
    </style>
</head>
<!doctype html>
<html lang="ar" dir="rtl">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width,initial-scale=1" />
<title>تأكيد بريدك الإلكتروني</title>
<style>
  /* إعدادات عامة مصغّرة وRTL */
  body{margin:0;background:#f7f9f8;color:#1f2937;font-family:Tahoma,"Segoe UI",Arial,sans-serif;direction:rtl;text-align:right;}
  .email-wrapper{max-width:600px;margin:0 auto;padding:16px;}
  /* الهيدر – ألوان الهوية */
  .header{background:#2f7d6d;color:#fff;border-radius:12px;padding:16px;text-align:center;}
  .logo{font-weight:700;font-size:16px;line-height:1.2;}
  .logo-subtitle{opacity:.9;font-size:11px;margin-top:4px;}
  .header h1{margin:10px 0 0;font-size:16px;font-weight:700;line-height:1.3;}
  /* المحتوى */
  .content{background:#fff;border:1px solid #e6e9ec;border-radius:12px;margin-top:12px;padding:14px;}
  .greeting{font-size:13px;font-weight:700;margin-bottom:8px;color:#1f2937;}
  .message{font-size:12px;line-height:1.5;margin:0 0 10px;color:#3b4451;}
  /* صندوق الرمز */
  .code-container{background:#e9f7f1;border:2px dashed #2f7d6d;border-radius:12px;padding:12px;text-align:center;margin:12px 0;}
  .code-label{font-size:11px;color:#256b5d;margin-bottom:6px;}
  .code{font-size:22px;font-weight:800;letter-spacing:6px;color:#134e4a;font-family:Consolas,Menlo,Monaco,monospace;}
  .notice{background:#fff6e6;border-right:3px solid #f1b75b;border-radius:8px;padding:10px;margin-top:10px;}
  .notice p{margin:0;font-size:11px;color:#6b5e3c;}
  /* الفوتر */
  .footer{color:#3b4451;text-align:center;margin-top:14px;padding:10px 6px;}
  .footer-brand{font-weight:700;color:#2f7d6d;font-size:12px;margin-bottom:4px;}
  .footer-text{font-size:10px;color:#6c757d;margin:3px 0;}
  .social-links{font-size:10px;margin-top:6px;color:#2f7d6d}
  .social-links a{color:#2f7d6d;text-decoration:none}
  .social-links a:hover{text-decoration:underline}
  /* تصغير المسافات على الشاشات الصغيرة */
  @media (max-width:480px){
    .email-wrapper{padding:10px}
    .header{padding:12px}
    .content{padding:12px}
    .code{font-size:20px;letter-spacing:5px}
  }
</style>
</head>
<body>
  <div class="email-wrapper">
    <div class="header">
      <div class="logo">لوفت الدغيري</div>
      <div class="logo-subtitle">Al-Dughairi Loft</div>
      <h1>تأكيد بريدك الإلكتروني</h1>
    </div>

    <div class="content">
      <div class="greeting">مرحباً بك {{.Name}}</div>

      <p class="message">
        شكراً لانضمامك إلى لوفت الدغيري. لإكمال عملية التحقق من بريدك الإلكتروني،
        يُرجى استخدام رمز التفعيل التالي:
      </p>

      <div class="code-container">
        <div class="code-label">رمز التفعيل</div>
        <div class="code">{{.verification_code}}</div>
      </div>

      <div class="notice">
        <p>الرمز صالح لمدة {{.expires_in}}. يُرجى عدم مشاركة الرمز مع أي شخص.</p>
      </div>

      <p class="message" style="font-size:11px;color:#6c757d;margin-top:10px;">
        إذا لم تقم بإنشاء حساب في لوفت الدغيري، يمكنك تجاهل هذه الرسالة بأمان.
      </p>
    </div>

    <div class="footer">
      <div class="footer-brand">لوفت الدغيري</div>
      <p class="footer-text">منصة متخصصة في بيع الحمام الزاجل والمزادات المباشرة</p>
      <div class="social-links">
        <a href="https://dughairiloft.com/" target="_blank">الموقع الإلكتروني</a> •
        <a href="mailto:contact@dughairiloft.com" target="_blank">تواصل معنا</a> •
        <a href="https://dughairiloft.com/terms" target="_blank">الشروط والأحكام</a>
      </div>
      <p class="footer-text" style="margin-top:8px;">&copy; 2025 لوفت الدغيري - جميع الحقوق محفوظة<br>القصيم - بريدة - المملكة العربية السعودية</p>
    </div>
  </div>
</body>
</html>

</html>`,
			"en": `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; background: #f9f9f9; }
        .header { background: #1a73e8; color: white; padding: 20px; text-align: center; border-radius: 10px 10px 0 0; }
        .content { background: white; padding: 30px; border-radius: 0 0 10px 10px; }
        .code { font-size: 24px; font-weight: bold; letter-spacing: 4px; background: #eef3fd; padding: 10px 15px; display: inline-block; border-radius: 6px; }
        .muted { color: #666; font-size: 12px; margin-top: 10px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Verify Your Email</h1>
        </div>
        <div class="content">
            <h2>Hello {{.Name}},</h2>
            <p>Use the verification code below to complete your email verification:</p>
            <p class="code">{{.verification_code}}</p>
            <p class="muted">The code is valid for {{.expires_in}}. If you didn't request this, you can ignore this email.</p>
            <p class="muted">&copy; 2025 Al-Dughairi Loft - All rights reserved.</p>
        </div>
    </div>
</body>
</html>`,
		},
		TextBody: map[string]string{
			"ar": `مرحباً بك {{.Name}},

استخدم رمز التفعيل التالي لإكمال التحقق من بريدك:

{{.verification_code}}

صالح لمدة {{.expires_in}}. إذا لم تطلب هذا الرمز فتجاهل الرسالة.`,
			"en": `Hello {{.Name}},

Use the verification code below to verify your email:

{{.verification_code}}

Valid for {{.expires_in}}. If you didn't request this code, please ignore this email.`,
		},
	},
	"verification_approved": {
		ID:          "verification_approved",
		Description: "إشعار بترقية الحساب إلى موثق",
		Subject: map[string]string{
			"ar": "تمت ترقية حسابك إلى موثق - لوفت الدغيري",
			"en": "Your Account Has Been Verified - Al-Dughairi Loft",
		},
		HTMLBody: map[string]string{
			"ar": `<!DOCTYPE html>
<html dir="rtl" lang="ar">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, 'Tajawal', sans-serif;
            line-height: 1.6;
            color: #333;
            direction: rtl;
            background: #f5f5f5;
        }
        .email-wrapper {
            max-width: 600px;
            margin: 20px auto;
            background: #ffffff;
            border-radius: 16px;
            overflow: hidden;
            box-shadow: 0 4px 20px rgba(0,0,0,0.08);
        }
        .header {
            background: linear-gradient(135deg, #2E7D6E 0%, #4A9B8E 100%);
            color: white;
            padding: 44px 30px;
            text-align: center;
        }
        .logo {
            font-size: 18px;
            margin-bottom: 8px;
        }
        .header h1 {
            font-size: 24px;
            font-weight: 800;
        }
        .content { padding: 36px 30px; }
        .greeting {
            font-size: 20px;
            color: #2E7D6E;
            margin-bottom: 16px;
            font-weight: 700;
            text-align: center;
        }
        .message {
            color: #555;
            margin-bottom: 20px;
            line-height: 1.8;
            font-size: 15px;
            text-align: center;
        }
        .status-badge {
            background: #e8f5f1;
            border: 2px solid #2E7D6E;
            border-radius: 12px;
            padding: 20px;
            text-align: center;
            margin: 24px 0;
        }
        .status-badge .title {
            color: #2E7D6E;
            font-size: 18px;
            font-weight: 700;
            margin-bottom: 4px;
        }
        .status-badge .subtitle {
            color: #6B7B8C;
            font-size: 14px;
        }
        .benefits {
            background: #f8f9fa;
            border-radius: 12px;
            padding: 20px;
            margin: 24px 0;
        }
        .benefits h3 {
            color: #4A9B8E;
            font-size: 17px;
            margin-bottom: 12px;
            text-align: center;
        }
        .benefits ul { margin: 0; padding-right: 18px; color: #555; }
        .cta-button {
            display: inline-block;
            background: linear-gradient(135deg, #2E7D6E 0%, #4A9B8E 100%);
            color: white;
            padding: 14px 36px;
            text-decoration: none;
            border-radius: 8px;
            font-weight: 600;
            margin: 16px 0;
            box-shadow: 0 4px 12px rgba(46, 125, 110, 0.3);
        }
        .footer {
            background: #f8f9fa;
            padding: 24px;
            text-align: center;
            border-top: 1px solid #e9ecef;
        }
        .footer-brand {
            color: #4A9B8E;
            font-weight: 700;
            font-size: 16px;
            margin-bottom: 8px;
        }
        .footer-text { color: #6c757d; font-size: 13px; line-height: 1.6; }
    </style>
</head>
<body>
    <div class="email-wrapper">
        <div class="header">
            <div class="logo">لوفت الدغيري</div>
            <h1>تمت ترقية حسابك</h1>
        </div>
        <div class="content">
            <div class="greeting">مرحباً بك {{.Name}}</div>
            <p class="message">نود إبلاغك بأن طلب التوثيق الخاص بك قد تمت الموافقة عليه.</p>

            <div class="status-badge">
                <div class="title">حساب موثّق</div>
                <div class="subtitle">Verified Account</div>
            </div>

            <div class="benefits">
                <h3>المزايا المتاحة لك</h3>
                <ul>
                    <li>المشاركة في المزادات وفق الشروط والسياسات</li>
                    <li>حدود مزايدة أعلى</li>
                    <li>ظهور حالة "موثّق" في ملفك</li>
                    <li>مصداقية أعلى داخل المجتمع</li>
                </ul>
            </div>

            <center>
                <a href="{{.ProfileURL}}" class="cta-button">عرض ملفك الشخصي</a>
            </center>

            <p class="message" style="font-size: 14px; color: #6c757d; margin-top: 20px;">
                شكراً لثقتك بنا ونتمنى لك تجربة موفقة.
            </p>
        </div>
        <div class="footer">
            <div class="footer-brand">لوفت الدغيري</div>
            <p class="footer-text">منصة متخصصة في بيع الحمام الزاجل والمزادات المباشرة</p>
            <p class="footer-text" style="margin-top: 12px;">
                &copy; 2025 لوفت الدغيري - جميع الحقوق محفوظة<br>
                القصيم - بريدة - المملكة العربية السعودية
            </p>
        </div>
    </div>
</body>
</html>`,
			"en": `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; background: #f9f9f9; }
        .header { background: #28a745; color: white; padding: 20px; text-align: center; border-radius: 10px 10px 0 0; }
        .content { background: white; padding: 30px; border-radius: 0 0 10px 10px; }
        .button { display: inline-block; padding: 12px 28px; background: #28a745; color: white; text-decoration: none; border-radius: 6px; margin-top: 16px; }
        .muted { color: #666; font-size: 12px; margin-top: 12px; text-align: center; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Your Account is Verified</h1>
        </div>
        <div class="content">
            <h2>Hello {{.Name}},</h2>
            <p>Your verification request has been approved.</p>
            <ul>
                <li>Participate in auctions per platform policies</li>
                <li>Higher bidding limits</li>
                <li>“Verified” status on your profile</li>
                <li>Increased credibility in the community</li>
            </ul>
            <a href="{{.ProfileURL}}" class="button">View Your Profile</a>
            <p class="muted">&copy; 2025 Al-Dughairi Loft - All rights reserved.</p>
        </div>
    </div>
</body>
</html>`,
		},
		TextBody: map[string]string{
			"ar": `مرحباً بك {{.Name}},

تمت الموافقة على طلب التوثيق. أصبح حسابك الآن موثّقاً ويمكنك الاستفادة من مزايا المنصة مثل المشاركة في المزادات وفق الشروط، وحدود مزايدة أعلى، وظهور حالة "موثّق" في ملفك.`,
			"en": `Hello {{.Name}},

Your account has been verified. You can now participate in auctions per platform terms, with higher bidding limits and a “Verified” status on your profile.`,
		},
	},
	"verification_requested_admin": {
		ID:          "verification_requested_admin",
		Description: "إشعار إداري - وصول طلب ترقية/توثيق جديد",
		Subject: map[string]string{
			"ar": "طلب توثيق جديد من {{.user_name}} (#{{.request_id}})",
		},
		HTMLBody: map[string]string{
			"ar": `<!DOCTYPE html>
<html dir="rtl" lang="ar">
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; line-height: 1.6; color: #333; direction: rtl; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; background: #f9f9f9; }
        .header { background: #1a73e8; color: white; padding: 16px; text-align: center; border-radius: 10px 10px 0 0; }
        .content { background: white; padding: 24px; border-radius: 0 0 10px 10px; }
        .meta { background: #eef3fd; padding: 12px; border-radius: 6px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h2>طلب توثيق جديد</h2>
        </div>
        <div class="content">
            <p>تم إنشاء طلب توثيق جديد من المستخدم:</p>
            <div class="meta">
                <p><strong>المستخدم:</strong> {{.user_name}} ({{.user_email}})</p>
                <p><strong>رقم الطلب:</strong> {{.request_id}}</p>
                <p><strong>ملاحظة المستخدم:</strong> {{.note}}</p>
            </div>
            <p>يرجى مراجعة لوحة الإدارة والموافقة/الرفض.</p>
        </div>
    </div>
</body>
</html>`,
		},
		TextBody: map[string]string{
			"ar": `إشعار إداري - طلب توثيق جديد

المستخدم: {{.user_name}} ({{.user_email}})
رقم الطلب: {{.request_id}}
ملاحظة: {{.note}}

يرجى مراجعة لوحة الإدارة.`,
		},
	},
	"order_confirmation": {
		ID:          "order_confirmation",
		Description: "تأكيد الطلب للمشتري بعد الدفع الناجح",
		Subject: map[string]string{
			"ar": "تم تأكيد طلبك #{{.order_id}} - لوفت الدغيري",
			"en": "Your Order #{{.order_id}} Confirmed - Al-Dughairi Loft",
		},
		HTMLBody: map[string]string{
			"ar": `<!DOCTYPE html>
<html dir="rtl" lang="ar">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, 'Tajawal', sans-serif;
            line-height: 1.6;
            color: #333;
            direction: rtl;
            background: #f5f5f5;
        }
        .email-wrapper {
            max-width: 600px;
            margin: 20px auto;
            background: #ffffff;
            border-radius: 16px;
            overflow: hidden;
            box-shadow: 0 4px 20px rgba(0,0,0,0.08);
        }
        .header {
            background: linear-gradient(135deg, #28a745 0%, #20893a 100%);
            color: white;
            padding: 40px 30px;
            text-align: center;
        }
        .logo {
            font-size: 24px;
            font-weight: 800;
            margin-bottom: 10px;
        }
        .logo-subtitle {
            font-size: 13px;
            opacity: 0.9;
            font-weight: 400;
        }
        .header h1 {
            font-size: 22px;
            margin-top: 20px;
            font-weight: 700;
        }
        .content {
            padding: 32px 30px;
            background: white;
        }
        .success-badge {
            text-align: center;
            margin: 24px 0;
            padding: 20px;
            background: linear-gradient(135deg, #e8f5e9 0%, #d4edda 100%);
            border-radius: 12px;
            border: 2px solid #28a745;
        }
        .success-badge .icon {
            font-size: 48px;
            margin-bottom: 8px;
        }
        .success-badge .title {
            color: #28a745;
            font-size: 20px;
            font-weight: 700;
            margin-bottom: 4px;
        }
        .success-badge .subtitle {
            color: #6B7B8C;
            font-size: 14px;
        }
        .order-details {
            background: #f8f9fa;
            border-radius: 12px;
            padding: 20px;
            margin: 24px 0;
        }
        .order-details h3 {
            color: #4A9B8E;
            font-size: 17px;
            margin-bottom: 16px;
            border-bottom: 2px solid #4A9B8E;
            padding-bottom: 8px;
        }
        .detail-row {
            display: flex;
            justify-content: space-between;
            padding: 10px 0;
            border-bottom: 1px solid #e9ecef;
        }
        .detail-row:last-child {
            border-bottom: none;
        }
        .detail-label {
            color: #6B7B8C;
            font-weight: 600;
        }
        .detail-value {
            color: #2E7D6E;
            font-weight: 700;
        }
        .total-row {
            background: #e8f5f1;
            padding: 14px;
            border-radius: 8px;
            margin-top: 12px;
        }
        .total-row .detail-value {
            color: #28a745;
            font-size: 18px;
        }
        .info-box {
            background: #fff9e6;
            border-right: 4px solid #ffc107;
            padding: 14px;
            margin: 20px 0;
            border-radius: 6px;
        }
        .info-box p {
            color: #856404;
            font-size: 14px;
            margin: 4px 0;
        }
        .cta-button {
            display: inline-block;
            background: linear-gradient(135deg, #4A9B8E 0%, #2E7D6E 100%);
            color: white;
            padding: 14px 36px;
            text-decoration: none;
            border-radius: 8px;
            font-weight: 600;
            margin: 16px 0;
            box-shadow: 0 4px 12px rgba(74,155,142,0.3);
        }
        .footer {
            background: #f8f9fa;
            padding: 24px;
            text-align: center;
            border-top: 1px solid #e9ecef;
        }
        .footer-brand {
            color: #4A9B8E;
            font-weight: 700;
            font-size: 16px;
            margin-bottom: 6px;
        }
        .footer-text {
            color: #6c757d;
            font-size: 13px;
            line-height: 1.6;
        }
        .social-links { margin: 12px 0; }
        .social-links a {
            display: inline-block;
            margin: 0 8px;
            color: #6B7B8C;
            text-decoration: none;
            font-size: 13px;
        }
    </style>
</head>
<body>
    <div class="email-wrapper">
        <div class="header">
            <div class="logo">لوفت الدغيري</div>
            <div class="logo-subtitle">Al-Dughairi Loft</div>
            <h1>تم تأكيد طلبك بنجاح</h1>
        </div>
        <div class="content">
            <div class="success-badge">
                <div class="icon">✅</div>
                <div class="title">تم استلام طلبك</div>
                <div class="subtitle">شكراً لثقتك بنا</div>
            </div>

            <p style="font-size: 16px; color: #555; margin-bottom: 20px; text-align: center;">
                مرحباً <strong>{{.name}}</strong>،<br>
                تم تأكيد طلبك ودفعه بنجاح. نحن نعمل على تجهيزه الآن.
            </p>

            <div class="order-details">
                <h3>📋 تفاصيل الطلب</h3>
                <div class="detail-row">
                    <span class="detail-label">رقم الطلب:</span>
                    <span class="detail-value">#{{.order_id}}</span>
                </div>
                <div class="detail-row">
                    <span class="detail-label">رقم الفاتورة:</span>
                    <span class="detail-value">#{{.invoice_id}}</span>
                </div>
                <div class="detail-row">
                    <span class="detail-label">حالة الدفع:</span>
                    <span class="detail-value" style="color: #28a745;">✓ مدفوع</span>
                </div>
                <div class="total-row detail-row">
                    <span class="detail-label" style="font-size: 16px;">الإجمالي المدفوع:</span>
                    <span class="detail-value">{{.grand_total}} ر.س</span>
                </div>
            </div>

            {{if .has_pigeons}}
            <div class="info-box">
                <p><strong>📍 معلومات الاستلام:</strong></p>
                <p>• يُستلم من: لوفت الدغيري - بريدة، القصيم</p>
                <p>• سيتم التواصل معك خلال 24 ساعة لتحديد موعد الاستلام</p>
                <p>• أوقات الاستلام: الأحد - الخميس (9ص - 6م)</p>
                <p>• للاستفسار: <a href="mailto:contact@dughairiloft.com">contact@dughairiloft.com</a></p>
            </div>
            {{else}}
            <div class="info-box">
                <p><strong>🚚 معلومات التوصيل:</strong></p>
                <p>• سيتم شحن طلبك خلال 1-2 يوم عمل</p>
                <p>• مدة التوصيل: 2-3 أيام عمل</p>
                <p>• سنرسل لك رسالة نصية برقم التتبع</p>
                <p>• للاستفسار: <a href="mailto:contact@dughairiloft.com">contact@dughairiloft.com</a></p>
            </div>
            {{end}}

            <center>
                <a href="{{.order_url}}" class="cta-button">عرض تفاصيل الطلب</a>
            </center>

            <p style="font-size: 14px; color: #6c757d; margin-top: 24px; text-align: center;">
                سنبقيك على اطلاع بحالة طلبك عبر البريد الإلكتروني والرسائل النصية.
            </p>
        </div>
        <div class="footer">
            <div class="footer-brand">لوفت الدغيري</div>
            <p class="footer-text">منصة متخصصة في بيع الحمام الزاجل والمزادات المباشرة</p>
            <div class="social-links">
                <a href="https://dughairiloft.com/">الموقع الإلكتروني</a> •
                <a href="mailto:contact@dughairiloft.com">تواصل معنا</a> •
                <a href="https://dughairiloft.com/terms">الشروط والأحكام</a>
            </div>
            <p class="footer-text" style="margin-top: 12px;">
                &copy; 2025 لوفت الدغيري - جميع الحقوق محفوظة<br>
                القصيم - بريدة - المملكة العربية السعودية
            </p>
        </div>
    </div>
</body>
</html>`,
			"en": `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; background: #f9f9f9; }
        .header { background: #28a745; color: white; padding: 20px; text-align: center; border-radius: 10px 10px 0 0; }
        .content { background: white; padding: 30px; border-radius: 0 0 10px 10px; }
        .order-details { background: #f8f9fa; padding: 15px; border-radius: 8px; margin: 20px 0; }
        .button { display: inline-block; padding: 12px 30px; background: #28a745; color: white; text-decoration: none; border-radius: 5px; margin-top: 20px; }
        .muted { color: #666; font-size: 12px; margin-top: 12px; text-align: center; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>✅ Order Confirmed</h1>
        </div>
        <div class="content">
            <h2>Hello {{.name}},</h2>
            <p>Your order has been confirmed and paid successfully.</p>
            <div class="order-details">
                <p><strong>Order #:</strong> {{.order_id}}</p>
                <p><strong>Invoice #:</strong> {{.invoice_id}}</p>
                <p><strong>Total Paid:</strong> {{.grand_total}} SAR</p>
            </div>
            <p>We'll keep you updated on your order status via email and SMS.</p>
            <a href="{{.order_url}}" class="button">View Order Details</a>
            <p class="muted">&copy; 2025 Al-Dughairi Loft - All rights reserved.</p>
        </div>
    </div>
</body>
</html>`,
		},
		TextBody: map[string]string{
			"ar": `مرحباً {{.name}},

تم تأكيد طلبك ودفعه بنجاح!

رقم الطلب: #{{.order_id}}
رقم الفاتورة: #{{.invoice_id}}
الإجمالي المدفوع: {{.grand_total}} ر.س

سنبقيك على اطلاع بحالة طلبك.

لعرض تفاصيل الطلب: {{.order_url}}

شكراً لثقتك بنا.`,
			"en": `Hello {{.name}},

Your order has been confirmed and paid successfully!

Order #: {{.order_id}}
Invoice #: {{.invoice_id}}
Total Paid: {{.grand_total}} SAR

We'll keep you updated on your order status.

View Order: {{.order_url}}

Thank you for your trust.`,
		},
	},
	"order_paid_admin": {
		ID:          "order_paid_admin",
		Description: "إشعار إداري - إتمام دفع طلب",
		Subject: map[string]string{
			"ar": "تم دفع الطلب #{{.order_id}} (فاتورة #{{.invoice_id}})",
		},
		HTMLBody: map[string]string{
			"ar": `<!DOCTYPE html>
<html dir="rtl" lang="ar">
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; line-height: 1.6; color: #333; direction: rtl; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; background: #f9f9f9; }
        .header { background: #28a745; color: white; padding: 16px; text-align: center; border-radius: 10px 10px 0 0; }
        .content { background: white; padding: 24px; border-radius: 0 0 10px 10px; }
        .meta { background: #e9f7ef; padding: 12px; border-radius: 6px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h2>تم تأكيد دفع طلب</h2>
        </div>
        <div class="content">
            <p>تم دفع الطلب بنجاح وتحديث حالته إلى "مدفوع".</p>
            <div class="meta">
                <p><strong>رقم الطلب:</strong> {{.order_id}}</p>
                <p><strong>رقم الفاتورة:</strong> {{.invoice_id}}</p>
                <p><strong>الإجمالي:</strong> {{.grand_total}} ر.س</p>
                <p><strong>العميل:</strong> {{.buyer_name}} ({{.buyer_email}})</p>
            </div>
            <p>يرجى استكمال إجراءات الشحن/التسليم حسب السياسة.</p>
        </div>
    </div>
</body>
</html>`,
		},
		TextBody: map[string]string{
			"ar": `إشعار إداري - تم دفع الطلب

رقم الطلب: {{.order_id}}
رقم الفاتورة: {{.invoice_id}}
الإجمالي: {{.grand_total}} ر.س
العميل: {{.buyer_name}} ({{.buyer_email}})

يرجى متابعة إجراءات الشحن/التسليم.`,
		},
	},
	"welcome": {
		ID:          "welcome",
		Description: "رسالة الترحيب بعد التسجيل",
		Subject: map[string]string{
			"ar": "مرحباً بك في لوفت الدغيري - منصة الحمام الزاجل",
			"en": "Welcome to Al-Dughairi Loft",
		},
		HTMLBody: map[string]string{
			"ar": `<!DOCTYPE html>
<html dir="rtl" lang="ar">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, 'Tajawal', sans-serif;
            line-height: 1.6;
            color: #333;
            direction: rtl;
            background: #f5f5f5;
        }
        .email-wrapper {
            max-width: 600px;
            margin: 20px auto;
            background: #ffffff;
            border-radius: 16px;
            overflow: hidden;
            box-shadow: 0 4px 20px rgba(0,0,0,0.08);
        }
        .header {
            background: linear-gradient(135deg, #4A9B8E 0%, #2E7D6E 100%);
            color: white;
            padding: 44px 30px;
            text-align: center;
        }
        .logo { font-size: 26px; font-weight: 800; margin-bottom: 8px; }
        .logo-subtitle { font-size: 14px; opacity: .95; margin-bottom: 14px; }
        .header h1 { font-size: 24px; font-weight: 700; margin-top: 6px; }
        .content { padding: 36px 30px; }
        .greeting { font-size: 20px; color: #2E7D6E; margin-bottom: 16px; font-weight: 700; }
        .message { color: #555; margin-bottom: 20px; line-height: 1.8; font-size: 15px; }
        .features { background: #f8f9fa; border-radius: 12px; padding: 20px; margin: 24px 0; }
        .features h3 { color: #4A9B8E; font-size: 17px; margin-bottom: 12px; }
        .features ul { margin: 0; padding-right: 18px; color: #555; }
        .cta-button {
            display: inline-block; background: linear-gradient(135deg, #4A9B8E 0%, #2E7D6E 100%);
            color: white; padding: 14px 36px; text-decoration: none; border-radius: 8px;
            font-weight: 600; margin: 16px 0; box-shadow: 0 4px 12px rgba(74,155,142,0.3);
        }
        .footer { background: #f8f9fa; padding: 24px; text-align: center; border-top: 1px solid #e9ecef; }
        .footer-brand { color: #4A9B8E; font-weight: 700; font-size: 16px; margin-bottom: 8px; }
        .footer-text { color: #6c757d; font-size: 13px; line-height: 1.6; }
    </style>
</head>
<body>
    <div class="email-wrapper">
        <div class="header">
            <div class="logo">لوفت الدغيري</div>
            <div class="logo-subtitle">Al-Dughairi Loft</div>
            <h1>أهلاً وسهلاً بك</h1>
        </div>
        <div class="content">
            <div class="greeting">مرحباً بك {{.Name}}</div>
            <p class="message">
                سعداء بانضمامك إلينا. حسابك جاهز للاستخدام والمشاركة في المزادات والاستفادة من خدمات المنصة.
            </p>
            <div class="features">
                <h3>ماذا يمكنك أن تفعل الآن؟</h3>
                <ul>
                    <li>استعراض المزادات المتاحة</li>
                    <li>المشاركة في المزايدات</li>
                    <li>شراء المستلزمات المرتبطة بالمنصة</li>
                    <li>التواصل مع المجتمع والخبراء</li>
                    <li>متابعة الأخبار والنصائح المتخصصة</li>
                </ul>
            </div>
            <center>
                <a href="{{.ActivationURL}}" class="cta-button">ابدأ الآن</a>
            </center>
            <p class="message" style="font-size: 14px; color: #6c757d; margin-top: 20px;">
                إذا كان لديك أي استفسار، فريق الدعم جاهز لمساعدتك.
            </p>
        </div>
        <div class="footer">
            <div class="footer-brand">لوفت الدغيري</div>
            <p class="footer-text">منصة متخصصة في بيع الحمام الزاجل والمزادات المباشرة</p>
            <p class="footer-text" style="margin-top: 12px;">
                &copy; 2025 لوفت الدغيري - جميع الحقوق محفوظة<br>
                القصيم - بريدة - المملكة العربية السعودية
            </p>
        </div>
    </div>
</body>
</html>`,
			"en": `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; background: #f9f9f9; }
        .header { background: #1a73e8; color: white; padding: 20px; text-align: center; border-radius: 10px 10px 0 0; }
        .content { background: white; padding: 30px; border-radius: 0 0 10px 10px; }
        .button { display: inline-block; padding: 12px 30px; background: #1a73e8; color: white; text-decoration: none; border-radius: 5px; margin-top: 20px; }
        .muted { color: #666; font-size: 12px; margin-top: 12px; text-align: center; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Welcome to Loft</h1>
        </div>
        <div class="content">
            <h2>Hello {{.Name}},</h2>
            <p>Your account is ready to use.</p>
            <p>You can now:</p>
            <ul>
                <li>Browse available auctions</li>
                <li>Participate in bidding</li>
                <li>Stay updated with news and tips</li>
            </ul>
            <a href="{{.ActivationURL}}" class="button">Activate Account</a>
            <p class="muted">&copy; 2025 Al-Dughairi Loft - All rights reserved.</p>
        </div>
    </div>
</body>
</html>`,
		},
		TextBody: map[string]string{
			"ar": `مرحباً بك {{.Name}},

سعداء بانضمامك. حسابك جاهز للاستخدام والمشاركة في المزادات والاستفادة من خدمات المنصة.

لتفعيل حسابك، يرجى زيارة: {{.ActivationURL}}`,
			"en": `Hello {{.Name}},

Welcome aboard! Your account is ready to use.

Activate your account: {{.ActivationURL}}`,
		},
	},
	"bid_placed": {
		ID:          "bid_placed",
		Description: "تأكيد وضع عرض سعر",
		Subject: map[string]string{
			"ar": "تم تسجيل عرضك بنجاح - {{.ItemName}}",
			"en": "Your Bid Has Been Placed - {{.ItemName}}",
		},
		HTMLBody: map[string]string{
			"ar": `<!DOCTYPE html>
<html dir="rtl" lang="ar">
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; line-height: 1.6; color: #333; direction: rtl; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; background: #f9f9f9; }
        .header { background: #28a745; color: white; padding: 20px; text-align: center; border-radius: 10px 10px 0 0; }
        .content { background: white; padding: 30px; border-radius: 0 0 10px 10px; }
        .bid-info { background: #f0f8ff; padding: 15px; border-radius: 5px; margin: 20px 0; }
        .button { display: inline-block; padding: 12px 30px; background: #1a73e8; color: white; text-decoration: none; border-radius: 5px; margin-top: 20px; }
        .muted { color: #666; font-size: 12px; margin-top: 12px; text-align: center; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>تم تسجيل عرضك بنجاح</h1>
        </div>
        <div class="content">
            <h2>مرحباً بك {{.Name}}</h2>
            <p>تم تسجيل عرضك على المزاد بنجاح.</p>
            <div class="bid-info">
                <h3>تفاصيل العرض:</h3>
                <p><strong>السلعة:</strong> {{.ItemName}}</p>
                <p><strong>قيمة العرض:</strong> {{.BidAmount}} ريال</p>
                <p><strong>وقت العرض:</strong> {{.BidTime}}</p>
                <p><strong>رقم المزاد:</strong> #{{.AuctionID}}</p>
            </div>
            <p>سنقوم بإشعارك في حال تم تجاوز عرضك أو عند انتهاء المزاد.</p>
            <a href="{{.AuctionURL}}" class="button">عرض المزاد</a>
            <p class="muted">&copy; 2025 لوفت الدغيري - جميع الحقوق محفوظة</p>
        </div>
    </div>
</body>
</html>`,
			"en": `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; background: #f9f9f9; }
        .header { background: #28a745; color: white; padding: 20px; text-align: center; border-radius: 10px 10px 0 0; }
        .content { background: white; padding: 30px; border-radius: 0 0 10px 10px; }
        .bid-info { background: #f0f8ff; padding: 15px; border-radius: 5px; margin: 20px 0; }
        .button { display: inline-block; padding: 12px 30px; background: #1a73e8; color: white; text-decoration: none; border-radius: 5px; margin-top: 20px; }
        .muted { color: #666; font-size: 12px; margin-top: 12px; text-align: center; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Your Bid Has Been Placed</h1>
        </div>
        <div class="content">
            <h2>Hello {{.Name}},</h2>
            <p>Your bid was successfully placed.</p>
            <div class="bid-info">
                <h3>Bid Details:</h3>
                <p><strong>Item:</strong> {{.ItemName}}</p>
                <p><strong>Bid Amount:</strong> {{.BidAmount}} SAR</p>
                <p><strong>Bid Time:</strong> {{.BidTime}}</p>
                <p><strong>Auction ID:</strong> #{{.AuctionID}}</p>
            </div>
            <p>We'll notify you if you are outbid or when the auction ends.</p>
            <a href="{{.AuctionURL}}" class="button">View Auction</a>
            <p class="muted">&copy; 2025 Al-Dughairi Loft - All rights reserved.</p>
        </div>
    </div>
</body>
</html>`,
		},
		TextBody: map[string]string{
			"ar": `مرحباً بك {{.Name}},

تم تسجيل عرضك على المزاد بنجاح.

تفاصيل العرض:
- السلعة: {{.ItemName}}
- قيمة العرض: {{.BidAmount}} ريال
- وقت العرض: {{.BidTime}}
- رقم المزاد: #{{.AuctionID}}

سنقوم بإشعارك في حال تم تجاوز عرضك أو عند انتهاء المزاد.

لعرض المزاد: {{.AuctionURL}}`,
			"en": `Hello {{.Name}},

Your bid has been successfully placed.

Bid Details:
- Item: {{.ItemName}}
- Bid Amount: {{.BidAmount}} SAR
- Bid Time: {{.BidTime}}
- Auction ID: #{{.AuctionID}}

We'll notify you if your bid is outbid or when the auction ends.

View Auction: {{.AuctionURL}}`,
		},
	},
	"auction_won": {
		ID:          "auction_won",
		Description: "تهنئة بالفوز بالمزاد",
		Subject: map[string]string{
			"ar": "مبروك! لقد فزت بالمزاد - {{.ItemName}}",
			"en": "Congratulations! You Won the Auction - {{.ItemName}}",
		},
		HTMLBody: map[string]string{
			"ar": `<!DOCTYPE html>
<html dir="rtl" lang="ar">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, 'Tajawal', sans-serif;
            line-height: 1.6;
            color: #333;
            direction: rtl;
            background: #f5f5f5;
        }
        .email-wrapper {
            max-width: 600px;
            margin: 20px auto;
            background: #ffffff;
            border-radius: 16px;
            overflow: hidden;
            box-shadow: 0 4px 20px rgba(0,0,0,0.08);
        }
        .header {
            background: linear-gradient(135deg, #ffc107 0%, #ff9800 100%);
            color: #fff;
            padding: 44px 30px;
            text-align: center;
        }
        .logo { font-size: 18px; margin-bottom: 8px; }
        .header h1 { font-size: 24px; font-weight: 800; }
        .content { padding: 36px 30px; }
        .greeting { font-size: 20px; color: #ff9800; margin-bottom: 16px; font-weight: 700; text-align: center; }
        .message { color: #555; margin-bottom: 20px; line-height: 1.8; font-size: 15px; text-align: center; }
        .win-box { background: #fff3cd; border: 1px solid #ffecb5; border-radius: 12px; padding: 20px; margin: 24px 0; }
        .win-box h3 { color: #ff9800; font-size: 17px; margin-bottom: 16px; font-weight: 700; text-align: center; }
        .detail-row { display: flex; justify-content: space-between; padding: 10px 0; border-bottom: 1px solid rgba(255,193,7,0.25); }
        .detail-row:last-child { border-bottom: none; }
        .detail-label { color: #6B7B8C; font-weight: 600; }
        .detail-value { color: #2E7D6E; font-weight: 700; }
        .steps-box { background: #f8f9fa; border-radius: 12px; padding: 20px; margin: 24px 0; }
        .steps-box h3 { color: #4A9B8E; font-size: 17px; margin-bottom: 12px; }
        .step { padding: 10px 0; display: flex; align-items: start; }
        .step-number { background: #4A9B8E; color: white; width: 28px; height: 28px; border-radius: 50%; display: flex; align-items: center; justify-content: center; font-weight: bold; margin-left: 12px; flex-shrink: 0; }
        .step-text { color: #555; flex: 1; }
        .cta-button { display: inline-block; background: linear-gradient(135deg, #2E7D6E 0%, #4A9B8E 100%); color: white; padding: 14px 40px; text-decoration: none; border-radius: 8px; font-weight: 700; font-size: 15px; margin: 16px 0; box-shadow: 0 4px 12px rgba(46,125,110,0.4); }
        .footer { background: #f8f9fa; padding: 24px; text-align: center; border-top: 1px solid #e9ecef; }
        .footer-brand { color: #4A9B8E; font-weight: 700; font-size: 16px; margin-bottom: 8px; }
        .footer-text { color: #6c757d; font-size: 13px; line-height: 1.6; }
    </style>
</head>
<body>
    <div class="email-wrapper">
        <div class="header">
            <div class="logo">لوفت الدغيري</div>
            <h1>لقد فزت بالمزاد</h1>
        </div>
        <div class="content">
            <div class="greeting">تهانينا {{.Name}}</div>
            <p class="message">يسعدنا إبلاغك بأن عرضك كان الأعلى.</p>

            <div class="win-box">
                <h3>تفاصيل الفوز</h3>
                <div class="detail-row">
                    <span class="detail-label">المنتج</span>
                    <span class="detail-value">{{.ItemName}}</span>
                </div>
                <div class="detail-row">
                    <span class="detail-label">السعر النهائي</span>
                    <span class="detail-value">{{.FinalPrice}} ريال</span>
                </div>
                <div class="detail-row">
                    <span class="detail-label">رقم المزاد</span>
                    <span class="detail-value">#{{.AuctionID}}</span>
                </div>
                <div class="detail-row">
                    <span class="detail-label">تاريخ الانتهاء</span>
                    <span class="detail-value">{{.EndDate}}</span>
                </div>
            </div>

            <div class="steps-box">
                <h3>الخطوات التالية</h3>
                <div class="step">
                    <div class="step-number">1</div>
                    <div class="step-text">سيتواصل معك البائع خلال 24 ساعة لترتيب التسليم</div>
                </div>
                <div class="step">
                    <div class="step-number">2</div>
                    <div class="step-text">يُرجى إتمام عملية الدفع خلال 48 ساعة للحفاظ على الفوز</div>
                </div>
                <div class="step">
                    <div class="step-number">3</div>
                    <div class="step-text">ترتيب استلام المنتج حسب الاتفاق مع البائع</div>
                </div>
            </div>

            <center>
                <a href="{{.PaymentURL}}" class="cta-button">إتمام الدفع الآن</a>
            </center>

            <p class="message" style="font-size: 14px; color: #6c757d; margin-top: 20px;">
                نتمنى لك تجربة موفقة.
            </p>
        </div>
        <div class="footer">
            <div class="footer-brand">لوفت الدغيري</div>
            <p class="footer-text">منصة متخصصة في بيع الحمام الزاجل والمزادات المباشرة</p>
            <p class="footer-text" style="margin-top: 12px;">
                &copy; 2025 لوفت الدغيري - جميع الحقوق محفوظة<br>
                القصيم - بريدة - المملكة العربية السعودية
            </p>
        </div>
    </div>
</body>
</html>`,
			"en": `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; background: #f9f9f9; }
        .header { background: #ffc107; color: #333; padding: 20px; text-align: center; border-radius: 10px 10px 0 0; }
        .content { background: white; padding: 30px; border-radius: 0 0 10px 10px; }
        .button { display: inline-block; padding: 12px 30px; background: #28a745; color: white; text-decoration: none; border-radius: 6px; margin-top: 16px; }
        .muted { color: #666; font-size: 12px; margin-top: 12px; text-align: center; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>You Won the Auction</h1>
        </div>
        <div class="content">
            <h2>Congratulations {{.Name}}!</h2>
            <p>Your bid was the highest.</p>
            <ul>
                <li><strong>Item:</strong> {{.ItemName}}</li>
                <li><strong>Final Price:</strong> {{.FinalPrice}} SAR</li>
                <li><strong>Auction ID:</strong> #{{.AuctionID}}</li>
                <li><strong>End Date:</strong> {{.EndDate}}</li>
            </ul>
            <ol>
                <li>The seller will contact you within 24 hours</li>
                <li>Please complete payment within 48 hours</li>
                <li>Arrange item pickup as agreed</li>
            </ol>
            <a href="{{.PaymentURL}}" class="button">Complete Payment</a>
            <p class="muted">&copy; 2025 Al-Dughairi Loft - All rights reserved.</p>
        </div>
    </div>
</body>
</html>`,
		},
		TextBody: map[string]string{
			"ar": `تهانينا {{.Name}},

عرضك كان الأعلى وفزت بالمزاد.

تفاصيل الفوز:
- السلعة: {{.ItemName}}
- السعر النهائي: {{.FinalPrice}} ريال
- رقم المزاد: #{{.AuctionID}}
- تاريخ الانتهاء: {{.EndDate}}

الخطوات التالية:
1) سيتواصل معك البائع خلال 24 ساعة
2) يُرجى إتمام الدفع خلال 48 ساعة
3) ترتيب الاستلام حسب الاتفاق

إتمام الدفع: {{.PaymentURL}}`,
			"en": `Congratulations {{.Name}},

Your bid was the highest.

Winning Details:
- Item: {{.ItemName}}
- Final Price: {{.FinalPrice}} SAR
- Auction ID: #{{.AuctionID}}
- End Date: {{.EndDate}}

Next Steps:
1) Seller will contact you within 24 hours
2) Complete payment within 48 hours
3) Arrange pickup as agreed

Payment: {{.PaymentURL}}`,
		},
	},
	"password_reset": {
		ID:          "password_reset",
		Description: "إعادة تعيين كلمة المرور",
		Subject: map[string]string{
			"ar": "إعادة تعيين كلمة المرور - لوفت الدغيري",
			"en": "Password Reset - Al-Dughairi Loft",
		},
		HTMLBody: map[string]string{
			"ar": `<!DOCTYPE html>
<html dir="rtl" lang="ar">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, 'Tajawal', sans-serif;
            line-height: 1.6;
            color: #333;
            direction: rtl;
            background: #f5f5f5;
        }
        .email-wrapper {
            max-width: 600px;
            margin: 20px auto;
            background: #ffffff;
            border-radius: 16px;
            overflow: hidden;
            box-shadow: 0 4px 20px rgba(0,0,0,0.08);
        }
        .header {
            background: linear-gradient(135deg, #6B7B8C 0%, #4A6B82 100%);
            color: white;
            padding: 44px 30px;
            text-align: center;
        }
        .logo { font-size: 18px; margin-bottom: 8px; }
        .header h1 { font-size: 22px; font-weight: 800; }
        .content { padding: 36px 30px; }
        .greeting { font-size: 18px; color: #6B7B8C; margin-bottom: 14px; font-weight: 600; }
        .message { color: #555; margin-bottom: 18px; line-height: 1.8; font-size: 15px; }
        .cta-button {
            display: inline-block; background: linear-gradient(135deg, #6B7B8C 0%, #4A6B82 100%);
            color: white; padding: 14px 36px; text-decoration: none; border-radius: 8px;
            font-weight: 700; font-size: 15px; margin: 16px 0; box-shadow: 0 4px 12px rgba(107,123,140,0.3);
        }
        .expiry-box { background: #f8f9fa; border-right: 4px solid #6B7B8C; padding: 14px 18px; border-radius: 6px; margin: 18px 0; }
        .expiry-box p { color: #555; font-size: 14px; margin: 4px 0; }
        .footer { background: #f8f9fa; padding: 24px; text-align: center; border-top: 1px solid #e9ecef; }
        .footer-brand { color: #4A9B8E; font-weight: 700; font-size: 16px; margin-bottom: 8px; }
        .footer-text { color: #6c757d; font-size: 13px; line-height: 1.6; }
    </style>
</head>
<body>
    <div class="email-wrapper">
        <div class="header">
            <div class="logo">لوفت الدغيري</div>
            <h1>إعادة تعيين كلمة المرور</h1>
        </div>
        <div class="content">
            <div class="greeting">مرحباً بك {{.Name}}</div>
            <p class="message">تلقّينا طلباً لإعادة تعيين كلمة المرور الخاصة بحسابك.</p>
            <p class="message">إذا كنت أنت من قام بهذا الطلب، يُرجى الضغط على الزر أدناه لإنشاء كلمة مرور جديدة:</p>
            <center>
                <a href="{{.ResetURL}}" class="cta-button">إعادة تعيين كلمة المرور</a>
            </center>
            <div class="expiry-box">
                <p><strong>صلاحية الرابط:</strong> ساعة واحدة فقط</p>
                <p><strong>ملاحظة:</strong> يمكن استخدام الرابط مرة واحدة فقط</p>
            </div>
            <p class="message" style="font-size: 14px; color: #6c757d;">
                إذا واجهت مشكلة، انسخ الرابط التالي والصقه في المتصفح:<br>
                <span style="word-break: break-all; color: #4A9B8E;">{{.ResetURL}}</span>
            </p>
        </div>
        <div class="footer">
            <div class="footer-brand">لوفت الدغيري</div>
            <p class="footer-text">منصة متخصصة في بيع الحمام الزاجل والمزادات المباشرة</p>
            <p class="footer-text" style="margin-top: 12px;">
                &copy; 2025 لوفت الدغيري - جميع الحقوق محفوظة<br>
                القصيم - بريدة - المملكة العربية السعودية
            </p>
        </div>
    </div>
</body>
</html>`,
			"en": `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; background: #f9f9f9; }
        .header { background: #dc3545; color: white; padding: 20px; text-align: center; border-radius: 10px 10px 0 0; }
        .content { background: white; padding: 30px; border-radius: 0 0 10px 10px; }
        .button { display: inline-block; padding: 12px 30px; background: #dc3545; color: white; text-decoration: none; border-radius: 5px; margin-top: 20px; }
        .muted { color: #666; font-size: 12px; margin-top: 12px; text-align: center; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Password Reset</h1>
        </div>
        <div class="content">
            <h2>Hello {{.Name}},</h2>
            <p>We received a request to reset your account password.</p>
            <p>If this was you, click the button below to set a new password:</p>
            <a href="{{.ResetURL}}" class="button">Reset Password</a>
            <p class="muted">Link is valid for 1 hour and can be used once. &copy; 2025 Al-Dughairi Loft.</p>
        </div>
    </div>
</body>
</html>`,
		},
		TextBody: map[string]string{
			"ar": `مرحباً بك {{.Name}},

تلقّينا طلباً لإعادة تعيين كلمة المرور الخاصة بحسابك.

لإعادة التعيين، يُرجى زيارة: {{.ResetURL}}

تنبيه: الرابط صالح لمدة ساعة واحدة، ويُستخدم مرة واحدة فقط.`,
			"en": `Hello {{.Name}},

We received a request to reset your account password.

To reset your password, visit: {{.ResetURL}}

Note: Link is valid for one hour and single-use only.`,
		},
	},
	"bid_outbid": {
		ID:          "bid_outbid",
		Description: "إشعار بتجاوز عرض المزايدة",
		Subject: map[string]string{
			"ar": "تم تجاوز عرضك - {{.product_title}}",
			"en": "You've Been Outbid - {{.product_title}}",
		},
		HTMLBody: map[string]string{
			"ar": `<!DOCTYPE html>
<html dir="rtl" lang="ar">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, 'Tajawal', sans-serif;
            line-height: 1.6;
            color: #333;
            direction: rtl;
            background: #f5f5f5;
        }
        .email-wrapper {
            max-width: 600px;
            margin: 20px auto;
            background: #ffffff;
            border-radius: 16px;
            overflow: hidden;
            box-shadow: 0 4px 20px rgba(0,0,0,0.08);
        }
        .header {
            background: linear-gradient(135deg, #ff9800 0%, #ff6f00 100%);
            color: white;
            padding: 44px 30px;
            text-align: center;
        }
        .logo { font-size: 18px; margin-bottom: 8px; }
        .header h1 { font-size: 22px; font-weight: 800; }
        .content { padding: 36px 30px; }
        .greeting { font-size: 18px; color: #ff6f00; margin-bottom: 16px; font-weight: 700; }
        .message { color: #555; margin-bottom: 20px; line-height: 1.8; font-size: 15px; }
        .auction-box {
            background: #fff3e0;
            border: 2px solid #ff9800;
            border-radius: 12px;
            padding: 20px;
            margin: 24px 0;
        }
        .auction-box h3 {
            color: #ff6f00;
            font-size: 17px;
            margin-bottom: 16px;
            font-weight: 700;
            text-align: center;
        }
        .detail-row {
            display: flex;
            justify-content: space-between;
            padding: 10px 0;
            border-bottom: 1px solid rgba(255,152,0,0.25);
        }
        .detail-row:last-child { border-bottom: none; }
        .detail-label { color: #6B7B8C; font-weight: 600; }
        .detail-value { color: #2E7D6E; font-weight: 700; }
        .cta-button {
            display: inline-block;
            background: linear-gradient(135deg, #2E7D6E 0%, #4A9B8E 100%);
            color: white;
            padding: 14px 36px;
            text-decoration: none;
            border-radius: 8px;
            font-weight: 700;
            font-size: 15px;
            margin: 16px 0;
            box-shadow: 0 4px 12px rgba(46,125,110,0.4);
        }
        .footer {
            background: #f8f9fa;
            padding: 24px;
            text-align: center;
            border-top: 1px solid #e9ecef;
        }
        .footer-brand { color: #4A9B8E; font-weight: 700; font-size: 16px; margin-bottom: 8px; }
        .footer-text { color: #6c757d; font-size: 13px; line-height: 1.6; }
    </style>
</head>
<body>
    <div class="email-wrapper">
        <div class="header">
            <div class="logo">لوفت الدغيري</div>
            <h1>تم تجاوز عرضك</h1>
        </div>
        <div class="content">
            <div class="greeting">مرحباً بك {{.Name}}</div>
            <p class="message">
                للأسف، تم تجاوز عرضك في المزاد. يمكنك المزايدة مرة أخرى إذا كنت لا تزال مهتماً.
            </p>

            <div class="auction-box">
                <h3>تفاصيل المزاد</h3>
                <div class="detail-row">
                    <span class="detail-label">المنتج</span>
                    <span class="detail-value">{{.product_title}}</span>
                </div>
                <div class="detail-row">
                    <span class="detail-label">عرضك السابق</span>
                    <span class="detail-value">{{.your_bid}} ريال</span>
                </div>
                <div class="detail-row">
                    <span class="detail-label">العرض الحالي</span>
                    <span class="detail-value">{{.new_price}} ريال</span>
                </div>
                <div class="detail-row">
                    <span class="detail-label">رقم المزاد</span>
                    <span class="detail-value">#{{.auction_id}}</span>
                </div>
            </div>

            <center>
                <a href="{{.AuctionURL}}" class="cta-button">زايد الآن</a>
            </center>

            <p class="message" style="font-size: 14px; color: #6c757d; margin-top: 20px;">
                نتمنى لك حظاً موفقاً.
            </p>
        </div>
        <div class="footer">
            <div class="footer-brand">لوفت الدغيري</div>
            <p class="footer-text">منصة متخصصة في بيع الحمام الزاجل والمزادات المباشرة</p>
            <p class="footer-text" style="margin-top: 12px;">
                &copy; 2025 لوفت الدغيري - جميع الحقوق محفوظة<br>
                القصيم - بريدة - المملكة العربية السعودية
            </p>
        </div>
    </div>
</body>
</html>`,
			"en": `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; background: #f9f9f9; }
        .header { background: #ff9800; color: white; padding: 20px; text-align: center; border-radius: 10px 10px 0 0; }
        .content { background: white; padding: 30px; border-radius: 0 0 10px 10px; }
        .button {
            display: inline-block;
            padding: 12px 30px;
            background: #2E7D6E;
            color: white;
            text-decoration: none;
            border-radius: 6px;
            margin-top: 16px;
        }
        .muted { color: #666; font-size: 12px; margin-top: 12px; text-align: center; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>You've Been Outbid</h1>
        </div>
        <div class="content">
            <h2>Hello {{.Name}},</h2>
            <p>Unfortunately, your bid has been outbid. You can place a new bid if you're still interested.</p>
            <ul>
                <li><strong>Item:</strong> {{.product_title}}</li>
                <li><strong>Your Bid:</strong> {{.your_bid}} SAR</li>
                <li><strong>Current Price:</strong> {{.new_price}} SAR</li>
                <li><strong>Auction ID:</strong> #{{.auction_id}}</li>
            </ul>
            <a href="{{.AuctionURL}}" class="button">Bid Again</a>
            <p class="muted">&copy; 2025 Al-Dughairi Loft - All rights reserved.</p>
        </div>
    </div>
</body>
</html>`,
		},
		TextBody: map[string]string{
			"ar": `مرحباً بك {{.Name}},

للأسف، تم تجاوز عرضك في المزاد.

تفاصيل المزاد:
- المنتج: {{.product_title}}
- عرضك السابق: {{.your_bid}} ريال
- العرض الحالي: {{.new_price}} ريال
- رقم المزاد: #{{.auction_id}}

يمكنك المزايدة مرة أخرى: {{.AuctionURL}}`,
			"en": `Hello {{.Name}},

Unfortunately, your bid has been outbid.

Auction Details:
- Item: {{.product_title}}
- Your Bid: {{.your_bid}} SAR
- Current Price: {{.new_price}} SAR
- Auction ID: #{{.auction_id}}

Bid again: {{.AuctionURL}}`,
		},
	},
	"auction_ended_winner": {
		ID:          "auction_ended_winner",
		Description: "إشعار فوز بالمزاد",
		Subject: map[string]string{
			"ar": "🎉 مبروك! لقد فزت بالمزاد - {{.product_title}}",
			"en": "🎉 Congratulations! You Won the Auction - {{.product_title}}",
		},
		HTMLBody: map[string]string{
			"ar": `<!DOCTYPE html>
<html dir="rtl" lang="ar">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, 'Tajawal', sans-serif;
            line-height: 1.6;
            color: #333;
            direction: rtl;
            background: #f5f5f5;
        }
        .email-wrapper {
            max-width: 600px;
            margin: 20px auto;
            background: #ffffff;
            border-radius: 16px;
            overflow: hidden;
            box-shadow: 0 4px 20px rgba(0,0,0,0.08);
        }
        .header {
            background: linear-gradient(135deg, #2E7D6E 0%, #4A9B8E 100%);
            color: white;
            padding: 44px 30px;
            text-align: center;
        }
        .trophy { font-size: 48px; margin-bottom: 16px; }
        .header h1 { font-size: 24px; font-weight: 800; margin-bottom: 8px; }
        .content { padding: 36px 30px; }
        .greeting { font-size: 18px; color: #2E7D6E; margin-bottom: 16px; font-weight: 700; }
        .message { color: #555; margin-bottom: 20px; line-height: 1.8; font-size: 15px; }
        .winner-box {
            background: linear-gradient(135deg, #d4af37 0%, #f4d03f 100%);
            border-radius: 12px;
            padding: 24px;
            margin: 24px 0;
            text-align: center;
            color: #fff;
        }
        .winner-box h3 {
            font-size: 20px;
            margin-bottom: 12px;
            font-weight: 800;
        }
        .winning-amount {
            font-size: 32px;
            font-weight: 900;
            margin: 16px 0;
        }
        .detail-row {
            display: flex;
            justify-content: space-between;
            padding: 12px 0;
            border-bottom: 1px solid #e9ecef;
        }
        .detail-row:last-child { border-bottom: none; }
        .detail-label { color: #6B7B8C; font-weight: 600; }
        .detail-value { color: #2E7D6E; font-weight: 700; }
        .cta-button {
            display: inline-block;
            background: linear-gradient(135deg, #2E7D6E 0%, #4A9B8E 100%);
            color: white;
            padding: 16px 48px;
            text-decoration: none;
            border-radius: 8px;
            font-weight: 700;
            font-size: 16px;
            margin: 24px 0;
            box-shadow: 0 4px 16px rgba(46,125,110,0.4);
        }
        .warning-box {
            background: #fff3cd;
            border: 2px solid #ffc107;
            border-radius: 8px;
            padding: 16px;
            margin: 20px 0;
            color: #856404;
        }
        .footer {
            background: #f8f9fa;
            padding: 24px;
            text-align: center;
            border-top: 1px solid #e9ecef;
        }
        .footer-brand { color: #4A9B8E; font-weight: 700; font-size: 16px; margin-bottom: 8px; }
        .footer-text { color: #6c757d; font-size: 13px; line-height: 1.6; }
    </style>
</head>
<body>
    <div class="email-wrapper">
        <div class="header">
            <div class="trophy">🏆</div>
            <h1>مبروك! لقد فزت بالمزاد</h1>
        </div>
        <div class="content">
            <div class="greeting">عزيزي {{.name}}</div>
            <p class="message">
                نبارك لك فوزك في المزاد! لقد كنت أعلى مزايد وحصلت على المنتج.
            </p>

            <div class="winner-box">
                <h3>{{.product_title}}</h3>
                <div class="winning-amount">{{.winning_amount}} ريال</div>
                <p style="font-size: 14px; opacity: 0.9;">المبلغ النهائي</p>
            </div>

            <div style="background: #f8f9fa; padding: 20px; border-radius: 8px; margin: 20px 0;">
                <div class="detail-row">
                    <span class="detail-label">رقم المزاد</span>
                    <span class="detail-value">#{{.auction_id}}</span>
                </div>
                <div class="detail-row">
                    <span class="detail-label">رقم الطلب</span>
                    <span class="detail-value">#{{.order_id}}</span>
                </div>
                <div class="detail-row">
                    <span class="detail-label">رقم الفاتورة</span>
                    <span class="detail-value">{{.invoice_number}}</span>
                </div>
            </div>

            <div class="warning-box">
                <strong>⚠️ مهم:</strong> يرجى إتمام عملية الدفع خلال 48 ساعة وإلا سيتم إلغاء الطلب.
            </div>

            <center>
                <a href="{{.payment_url}}" class="cta-button">ادفع الآن</a>
            </center>

            <p class="message" style="font-size: 14px; color: #6c757d; margin-top: 24px;">
                شكراً لثقتك في منصة لوفت الدغيري. نتمنى لك تجربة موفقة.
            </p>
        </div>
        <div class="footer">
            <div class="footer-brand">لوفت الدغيري</div>
            <p class="footer-text">منصة متخصصة في بيع الحمام الزاجل والمزادات المباشرة</p>
            <p class="footer-text" style="margin-top: 12px;">
                &copy; 2025 لوفت الدغيري - جميع الحقوق محفوظة<br>
                القصيم - بريدة - المملكة العربية السعودية
            </p>
        </div>
    </div>
</body>
</html>`,
			"en": `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: 'Segoe UI', sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; background: #f9f9f9; }
        .header { background: #2E7D6E; color: white; padding: 30px; text-align: center; border-radius: 10px 10px 0 0; }
        .content { background: white; padding: 30px; border-radius: 0 0 10px 10px; }
        .winner-box { background: linear-gradient(135deg, #d4af37, #f4d03f); color: white; padding: 20px; border-radius: 8px; text-align: center; margin: 20px 0; }
        .button {
            display: inline-block;
            padding: 14px 40px;
            background: #2E7D6E;
            color: white;
            text-decoration: none;
            border-radius: 6px;
            margin-top: 16px;
            font-weight: bold;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🏆 Congratulations! You Won!</h1>
        </div>
        <div class="content">
            <h2>Dear {{.name}},</h2>
            <p>Congratulations on winning the auction!</p>
            <div class="winner-box">
                <h3>{{.product_title}}</h3>
                <h2>{{.winning_amount}} SAR</h2>
            </div>
            <ul>
                <li><strong>Auction ID:</strong> #{{.auction_id}}</li>
                <li><strong>Order ID:</strong> #{{.order_id}}</li>
                <li><strong>Invoice:</strong> {{.invoice_number}}</li>
            </ul>
            <p><strong>⚠️ Important:</strong> Please complete payment within 48 hours.</p>
            <a href="{{.payment_url}}" class="button">Pay Now</a>
        </div>
    </div>
</body>
</html>`,
		},
		TextBody: map[string]string{
			"ar": `مبروك {{.name}}!

لقد فزت بالمزاد:
- المنتج: {{.product_title}}
- المبلغ النهائي: {{.winning_amount}} ريال
- رقم المزاد: #{{.auction_id}}
- رقم الطلب: #{{.order_id}}
- رقم الفاتورة: {{.invoice_number}}

⚠️ مهم: يرجى إتمام الدفع خلال 48 ساعة.

ادفع الآن: {{.payment_url}}`,
			"en": `Congratulations {{.name}}!

You won the auction:
- Item: {{.product_title}}
- Final Amount: {{.winning_amount}} SAR
- Auction ID: #{{.auction_id}}
- Order ID: #{{.order_id}}
- Invoice: {{.invoice_number}}

⚠️ Important: Please complete payment within 48 hours.

Pay now: {{.payment_url}}`,
		},
	},
	"auction_ended_reserve_not_met": {
		ID:          "auction_ended_reserve_not_met",
		Description: "إشعار انتهاء المزاد - السعر الاحتياطي لم يتحقق",
		Subject: map[string]string{
			"ar": "انتهى المزاد - لم يتحقق السعر الاحتياطي - {{.product_title}}",
			"en": "Auction Ended - Reserve Not Met - {{.product_title}}",
		},
		HTMLBody: map[string]string{
			"ar": `<!DOCTYPE html>
<html dir="rtl" lang="ar">
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: 'Tajawal', sans-serif; line-height: 1.6; direction: rtl; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: #6B7B8C; color: white; padding: 20px; text-align: center; border-radius: 8px 8px 0 0; }
        .content { background: white; padding: 30px; border: 1px solid #ddd; border-top: none; }
        .info-box { background: #f8f9fa; padding: 15px; border-radius: 6px; margin: 15px 0; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>انتهى المزاد</h1>
        </div>
        <div class="content">
            <p>عزيزي {{.name}},</p>
            <p>للأسف، انتهى المزاد دون الوصول للسعر الاحتياطي.</p>
            <div class="info-box">
                <p><strong>المنتج:</strong> {{.product_title}}</p>
                <p><strong>أعلى عرض:</strong> {{.highest_bid}} ريال</p>
                <p><strong>رقم المزاد:</strong> #{{.auction_id}}</p>
            </div>
            <p>شكراً لمشاركتك. نتطلع لرؤيتك في مزادات قادمة.</p>
        </div>
    </div>
</body>
</html>`,
			"en": `Auction ended - reserve price not met. Thank you for participating.`,
		},
		TextBody: map[string]string{
			"ar": `عزيزي {{.name}},

للأسف، انتهى المزاد دون الوصول للسعر الاحتياطي.

- المنتج: {{.product_title}}
- أعلى عرض: {{.highest_bid}} ريال
- رقم المزاد: #{{.auction_id}}

شكراً لمشاركتك.`,
			"en": `Dear {{.name}},

Unfortunately, the auction ended without meeting the reserve price.

- Item: {{.product_title}}
- Highest Bid: {{.highest_bid}} SAR
- Auction ID: #{{.auction_id}}`,
		},
	},
	"auction_ended_lost": {
		ID:          "auction_ended_lost",
		Description: "إشعار خسارة المزاد",
		Subject: map[string]string{
			"ar": "انتهى المزاد - {{.product_title}}",
			"en": "Auction Ended - {{.product_title}}",
		},
		HTMLBody: map[string]string{
			"ar": `<!DOCTYPE html>
<html dir="rtl" lang="ar">
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: 'Tajawal', sans-serif; line-height: 1.6; direction: rtl; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: #6B7B8C; color: white; padding: 20px; text-align: center; }
        .content { background: white; padding: 30px; border: 1px solid #ddd; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>انتهى المزاد</h1>
        </div>
        <div class="content">
            <p>عزيزي {{.name}},</p>
            <p>للأسف، فاز مزايد آخر في المزاد: {{.product_title}}</p>
            <p><strong>رقم المزاد:</strong> #{{.auction_id}}</p>
            <p>نتطلع لرؤيتك في مزادات قادمة!</p>
        </div>
    </div>
</body>
</html>`,
			"en": `Auction ended. Another bidder won. Thank you for participating!`,
		},
		TextBody: map[string]string{
			"ar": `عزيزي {{.name}},

للأسف، فاز مزايد آخر في المزاد: {{.product_title}}

رقم المزاد: #{{.auction_id}}

نتطلع لرؤيتك في مزادات قادمة!`,
			"en": `Dear {{.name}},

Another bidder won the auction: {{.product_title}}

Auction ID: #{{.auction_id}}

We look forward to seeing you in future auctions!`,
		},
	},
}

// GetTemplate يجلب قالب البريد الإلكتروني
func GetTemplate(templateID string) (*EmailTemplate, error) {
	tmpl, exists := templates[templateID]
	if !exists {
		return nil, &errs.Error{Code: errs.NotFound, Message: "القالب غير موجود"}
	}
	return tmpl, nil
}

// RenderTemplate يقوم بتحويل القالب إلى HTML/Text باستخدام البيانات المعطاة
func RenderTemplate(templateID string, lang string, data TemplateData) (subject, html, text string, err error) {
	tmpl, err := GetTemplate(templateID)
	if err != nil {
		return "", "", "", err
	}

	// Default to Arabic if language not found
	if lang == "" {
		lang = "ar"
	}

	// Get subject
	subject = tmpl.Subject[lang]
	if subject == "" {
		subject = tmpl.Subject["ar"] // fallback to Arabic
	}

	// Render subject with data
	subjectTmpl, err := template.New("subject").Parse(subject)
	if err == nil {
		var subjectBuf bytes.Buffer
		if err := subjectTmpl.Execute(&subjectBuf, data); err == nil {
			subject = subjectBuf.String()
		}
	}

	// Get HTML body
	htmlBody := tmpl.HTMLBody[lang]
	if htmlBody == "" {
		htmlBody = tmpl.HTMLBody["ar"] // fallback to Arabic
	}

	// Render HTML with data
	htmlTmpl, err := template.New("html").Parse(htmlBody)
	if err != nil {
		return "", "", "", &errs.Error{Code: errs.Internal, Message: "فشل تحليل قالب HTML"}
	}
	var htmlBuf bytes.Buffer
	if err := htmlTmpl.Execute(&htmlBuf, data); err != nil {
		return "", "", "", &errs.Error{Code: errs.Internal, Message: "فشل تنفيذ قالب HTML"}
	}
	html = htmlBuf.String()

	// Get text body
	textBody := tmpl.TextBody[lang]
	if textBody == "" {
		textBody = tmpl.TextBody["ar"] // fallback to Arabic
	}

	// Render text with data
	textTmpl, err := template.New("text").Parse(textBody)
	if err != nil {
		return "", "", "", &errs.Error{Code: errs.Internal, Message: "فشل تحليل قالب النص"}
	}
	var textBuf bytes.Buffer
	if err := textTmpl.Execute(&textBuf, data); err != nil {
		return "", "", "", &errs.Error{Code: errs.Internal, Message: "فشل تنفيذ قالب النص"}
	}
	text = textBuf.String()

	return subject, html, text, nil
}

// GetAvailableTemplates يرجع قائمة بجميع القوالب المتاحة
func GetAvailableTemplates() []string {
	var ids []string
	for id := range templates {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// GetTemplateInfo يرجع معلومات عن قالب معين
func GetTemplateInfo(templateID string) (map[string]interface{}, error) {
	tmpl, err := GetTemplate(templateID)
	if err != nil {
		return nil, err
	}

	languages := []string{}
	for lang := range tmpl.Subject {
		languages = append(languages, lang)
	}
	sort.Strings(languages)

	return map[string]interface{}{
		"id":          tmpl.ID,
		"description": tmpl.Description,
		"languages":   languages,
	}, nil
}
