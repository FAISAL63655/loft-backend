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
	"welcome": {
		ID:          "welcome",
		Description: "رسالة الترحيب بعد التسجيل",
		Subject: map[string]string{
			"ar": "مرحباً بك في لوفت - منصة المزادات الرائدة",
			"en": "Welcome to Loft - The Leading Auction Platform",
		},
		HTMLBody: map[string]string{
			"ar": `<!DOCTYPE html>
<html dir="rtl" lang="ar">
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; line-height: 1.6; color: #333; direction: rtl; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; background: #f9f9f9; }
        .header { background: #1a73e8; color: white; padding: 20px; text-align: center; border-radius: 10px 10px 0 0; }
        .content { background: white; padding: 30px; border-radius: 0 0 10px 10px; }
        .button { display: inline-block; padding: 12px 30px; background: #1a73e8; color: white; text-decoration: none; border-radius: 5px; margin-top: 20px; }
        .footer { text-align: center; margin-top: 30px; font-size: 12px; color: #666; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>مرحباً بك في لوفت!</h1>
        </div>
        <div class="content">
            <h2>أهلاً {{.Name}}،</h2>
            <p>نحن سعداء بانضمامك إلى منصة لوفت للمزادات. حسابك جاهز الآن للاستخدام.</p>
            <p>يمكنك الآن:</p>
            <ul>
                <li>استعراض المزادات المتاحة</li>
                <li>المشاركة في المزايدات</li>
                <li>إنشاء مزاداتك الخاصة</li>
                <li>متابعة العناصر المفضلة</li>
            </ul>
            <a href="{{.ActivationURL}}" class="button">تفعيل الحساب</a>
            <div class="footer">
                <p>إذا لم تقم بإنشاء هذا الحساب، يرجى تجاهل هذه الرسالة.</p>
                <p>&copy; 2024 لوفت - جميع الحقوق محفوظة</p>
            </div>
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
        .footer { text-align: center; margin-top: 30px; font-size: 12px; color: #666; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Welcome to Loft!</h1>
        </div>
        <div class="content">
            <h2>Hello {{.Name}},</h2>
            <p>We're excited to have you join the Loft auction platform. Your account is now ready to use.</p>
            <p>You can now:</p>
            <ul>
                <li>Browse available auctions</li>
                <li>Participate in bidding</li>
                <li>Create your own auctions</li>
                <li>Follow favorite items</li>
            </ul>
            <a href="{{.ActivationURL}}" class="button">Activate Account</a>
            <div class="footer">
                <p>If you didn't create this account, please ignore this message.</p>
                <p>&copy; 2024 Loft - All rights reserved</p>
            </div>
        </div>
    </div>
</body>
</html>`,
		},
		TextBody: map[string]string{
			"ar": `مرحباً {{.Name}}،

نحن سعداء بانضمامك إلى منصة لوفت للمزادات. حسابك جاهز الآن للاستخدام.

يمكنك الآن:
- استعراض المزادات المتاحة
- المشاركة في المزايدات
- إنشاء مزاداتك الخاصة
- متابعة العناصر المفضلة

لتفعيل حسابك، يرجى زيارة: {{.ActivationURL}}

إذا لم تقم بإنشاء هذا الحساب، يرجى تجاهل هذه الرسالة.

مع تحيات فريق لوفت`,
			"en": `Hello {{.Name}},

We're excited to have you join the Loft auction platform. Your account is now ready to use.

You can now:
- Browse available auctions
- Participate in bidding
- Create your own auctions
- Follow favorite items

To activate your account, please visit: {{.ActivationURL}}

If you didn't create this account, please ignore this message.

Best regards,
The Loft Team`,
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
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>تم تسجيل عرضك بنجاح!</h1>
        </div>
        <div class="content">
            <h2>مرحباً {{.Name}}،</h2>
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
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Your Bid Has Been Placed!</h1>
        </div>
        <div class="content">
            <h2>Hello {{.Name}},</h2>
            <p>Your bid has been successfully placed on the auction.</p>
            <div class="bid-info">
                <h3>Bid Details:</h3>
                <p><strong>Item:</strong> {{.ItemName}}</p>
                <p><strong>Bid Amount:</strong> {{.BidAmount}} SAR</p>
                <p><strong>Bid Time:</strong> {{.BidTime}}</p>
                <p><strong>Auction ID:</strong> #{{.AuctionID}}</p>
            </div>
            <p>We'll notify you if your bid is outbid or when the auction ends.</p>
            <a href="{{.AuctionURL}}" class="button">View Auction</a>
        </div>
    </div>
</body>
</html>`,
		},
		TextBody: map[string]string{
			"ar": `مرحباً {{.Name}}،

تم تسجيل عرضك على المزاد بنجاح.

تفاصيل العرض:
- السلعة: {{.ItemName}}
- قيمة العرض: {{.BidAmount}} ريال
- وقت العرض: {{.BidTime}}
- رقم المزاد: #{{.AuctionID}}

سنقوم بإشعارك في حال تم تجاوز عرضك أو عند انتهاء المزاد.

لعرض المزاد: {{.AuctionURL}}

مع تحيات فريق لوفت`,
			"en": `Hello {{.Name}},

Your bid has been successfully placed on the auction.

Bid Details:
- Item: {{.ItemName}}
- Bid Amount: {{.BidAmount}} SAR
- Bid Time: {{.BidTime}}
- Auction ID: #{{.AuctionID}}

We'll notify you if your bid is outbid or when the auction ends.

View Auction: {{.AuctionURL}}

Best regards,
The Loft Team`,
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
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; line-height: 1.6; color: #333; direction: rtl; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; background: #f9f9f9; }
        .header { background: #ffc107; color: #333; padding: 20px; text-align: center; border-radius: 10px 10px 0 0; }
        .content { background: white; padding: 30px; border-radius: 0 0 10px 10px; }
        .win-info { background: #fff3cd; padding: 15px; border-radius: 5px; margin: 20px 0; border-left: 4px solid #ffc107; }
        .button { display: inline-block; padding: 12px 30px; background: #28a745; color: white; text-decoration: none; border-radius: 5px; margin-top: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🎉 مبروك! لقد فزت بالمزاد!</h1>
        </div>
        <div class="content">
            <h2>تهانينا {{.Name}}!</h2>
            <p>يسعدنا إبلاغك بأنك الفائز في المزاد.</p>
            <div class="win-info">
                <h3>تفاصيل الفوز:</h3>
                <p><strong>السلعة:</strong> {{.ItemName}}</p>
                <p><strong>السعر النهائي:</strong> {{.FinalPrice}} ريال</p>
                <p><strong>رقم المزاد:</strong> #{{.AuctionID}}</p>
                <p><strong>تاريخ الانتهاء:</strong> {{.EndDate}}</p>
            </div>
            <p>الخطوات التالية:</p>
            <ol>
                <li>سيتواصل معك البائع خلال 24 ساعة</li>
                <li>يرجى إتمام عملية الدفع خلال 48 ساعة</li>
                <li>ترتيب استلام السلعة حسب الاتفاق</li>
            </ol>
            <a href="{{.PaymentURL}}" class="button">إتمام الدفع</a>
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
        .win-info { background: #fff3cd; padding: 15px; border-radius: 5px; margin: 20px 0; border-left: 4px solid #ffc107; }
        .button { display: inline-block; padding: 12px 30px; background: #28a745; color: white; text-decoration: none; border-radius: 5px; margin-top: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🎉 Congratulations! You Won the Auction!</h1>
        </div>
        <div class="content">
            <h2>Congratulations {{.Name}}!</h2>
            <p>We're pleased to inform you that you're the winner of the auction.</p>
            <div class="win-info">
                <h3>Winning Details:</h3>
                <p><strong>Item:</strong> {{.ItemName}}</p>
                <p><strong>Final Price:</strong> {{.FinalPrice}} SAR</p>
                <p><strong>Auction ID:</strong> #{{.AuctionID}}</p>
                <p><strong>End Date:</strong> {{.EndDate}}</p>
            </div>
            <p>Next Steps:</p>
            <ol>
                <li>The seller will contact you within 24 hours</li>
                <li>Please complete payment within 48 hours</li>
                <li>Arrange item pickup as agreed</li>
            </ol>
            <a href="{{.PaymentURL}}" class="button">Complete Payment</a>
        </div>
    </div>
</body>
</html>`,
		},
		TextBody: map[string]string{
			"ar": `تهانينا {{.Name}}!

يسعدنا إبلاغك بأنك الفائز في المزاد.

تفاصيل الفوز:
- السلعة: {{.ItemName}}
- السعر النهائي: {{.FinalPrice}} ريال
- رقم المزاد: #{{.AuctionID}}
- تاريخ الانتهاء: {{.EndDate}}

الخطوات التالية:
1. سيتواصل معك البائع خلال 24 ساعة
2. يرجى إتمام عملية الدفع خلال 48 ساعة
3. ترتيب استلام السلعة حسب الاتفاق

لإتمام الدفع: {{.PaymentURL}}

مع تحيات فريق لوفت`,
			"en": `Congratulations {{.Name}}!

We're pleased to inform you that you're the winner of the auction.

Winning Details:
- Item: {{.ItemName}}
- Final Price: {{.FinalPrice}} SAR
- Auction ID: #{{.AuctionID}}
- End Date: {{.EndDate}}

Next Steps:
1. The seller will contact you within 24 hours
2. Please complete payment within 48 hours
3. Arrange item pickup as agreed

Complete Payment: {{.PaymentURL}}

Best regards,
The Loft Team`,
		},
	},
	"password_reset": {
		ID:          "password_reset",
		Description: "إعادة تعيين كلمة المرور",
		Subject: map[string]string{
			"ar": "إعادة تعيين كلمة المرور - لوفت",
			"en": "Password Reset - Loft",
		},
		HTMLBody: map[string]string{
			"ar": `<!DOCTYPE html>
<html dir="rtl" lang="ar">
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; line-height: 1.6; color: #333; direction: rtl; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; background: #f9f9f9; }
        .header { background: #dc3545; color: white; padding: 20px; text-align: center; border-radius: 10px 10px 0 0; }
        .content { background: white; padding: 30px; border-radius: 0 0 10px 10px; }
        .button { display: inline-block; padding: 12px 30px; background: #dc3545; color: white; text-decoration: none; border-radius: 5px; margin-top: 20px; }
        .warning { background: #fff3cd; padding: 10px; border-radius: 5px; margin-top: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>إعادة تعيين كلمة المرور</h1>
        </div>
        <div class="content">
            <h2>مرحباً {{.Name}}،</h2>
            <p>تلقينا طلباً لإعادة تعيين كلمة المرور الخاصة بحسابك.</p>
            <p>انقر على الزر أدناه لإعادة تعيين كلمة المرور:</p>
            <a href="{{.ResetURL}}" class="button">إعادة تعيين كلمة المرور</a>
            <div class="warning">
                <p><strong>تنبيه:</strong> هذا الرابط صالح لمدة ساعة واحدة فقط.</p>
                <p>إذا لم تطلب إعادة تعيين كلمة المرور، يرجى تجاهل هذه الرسالة.</p>
            </div>
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
        .warning { background: #fff3cd; padding: 10px; border-radius: 5px; margin-top: 20px; }
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
            <p>Click the button below to reset your password:</p>
            <a href="{{.ResetURL}}" class="button">Reset Password</a>
            <div class="warning">
                <p><strong>Note:</strong> This link is valid for 1 hour only.</p>
                <p>If you didn't request a password reset, please ignore this message.</p>
            </div>
        </div>
    </div>
</body>
</html>`,
		},
		TextBody: map[string]string{
			"ar": `مرحباً {{.Name}}،

تلقينا طلباً لإعادة تعيين كلمة المرور الخاصة بحسابك.

لإعادة تعيين كلمة المرور، يرجى زيارة: {{.ResetURL}}

تنبيه: هذا الرابط صالح لمدة ساعة واحدة فقط.

إذا لم تطلب إعادة تعيين كلمة المرور، يرجى تجاهل هذه الرسالة.

مع تحيات فريق لوفت`,
			"en": `Hello {{.Name}},

We received a request to reset your account password.

To reset your password, please visit: {{.ResetURL}}

Note: This link is valid for 1 hour only.

If you didn't request a password reset, please ignore this message.

Best regards,
The Loft Team`,
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
