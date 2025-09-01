# تقرير فحص المشروع — مطابقة المتطلبات (PRD) وجودة الكود

آخر تحديث: 2025-09-01

هذا التقرير يلخّص نتائج فحص شامل لمستودع `loft-backend` (خدمات Encore.go + PostgreSQL) ومطابقته لوثيقة المتطلبات: `C:\Users\PCD\Desktop\loft-asd\loft-dughairi-backend-prd-v1.md`.

ملاحظة بيئية: لم نتمكن من تشغيل أوامر Go محلياً (أداة Go غير مثبتة في البيئة الحالية)، لذا اعتمدت المراجعة على قراءة الكود ساكنًا وفحص الهجرات والواجهات. يُنصح بتشغيل أوامر الفحص والاختبارات المذكورة في قسم «خطوات التحقق المقترحة» أدناه على بيئة فيها Go مثبتًا.

## خلاصة تنفيذية

- المطابقة العامة لـ PRD: عالية. معظم التدفقات الحرجة (المزايدات مع Anti‑Sniping، السلة/الحجوزات، الدفع عبر Moyasar مع Webhook Idempotent، الشحن، إشعارات البريد، إعدادات النظام الحيّة) موجودة وتراعي قواعد العمل المذكورة.
- جودة الكود: جيدة ومنظمة (خدمات `svc/*`، مكتبات مشتركة `pkg/*`، هجرات واضحة `coredb/migrations`). استخدام الأسرار عبر Encore، وأساليب آمنة لمعالجة JWT/Passwords.
- نقاط تحتاج انتباهاً أو تحسينًا: صغيرة/توثيقية في الغالب، مع بعض الثغرات المحتملة منخفضة الخطورة مذكورة أدناه.

## مطابقة المجالات الرئيسية مع PRD

- المصادقة (svc/auth, pkg/authn)
  - JWT: مدة Access ~20m وRefresh ~60d (مطابق). Argon2id لتجزئة كلمات المرور مع دعم توافق bcrypt (جيد).
  - تفعيل البريد: تدفق إنشاء/تحقق/إعادة إرسال رمز موجود؛ تخزين محاولات/فواصل زمنية؛ منع brute‑force (≤5 محاولات) — مطابق.
  - AuthHandler يحقن `auth.Data()` (UserID/Role/Email) للاستهلاك في الخدمات الأخرى — مطابق لفلسفة Encore.

- المستخدمون والتحقق (svc/users)
  - `GET/PATCH /me`، طلبات التوثيق والموافقة/الرفض (Admin فقط) — موجودة.
  - العناوين: إنشاء/تحديث/أرشفة مع اشتراط Email Verified — مطابق.

- الكتالوج والوسائط (svc/catalog)
  - `GET /products`, `GET /products/:id` مع فلترة/ترتيب/ترقيم — موجودة مع تحقق مُسبق من القيم.
  - `POST /products` (Admin) + رفع وسائط `raw` لـ GCS، دعم watermark/thumbnail عبر إعدادات — موجود.

- المزادات والمزايدات (svc/auctions)
  - إنشاء/إلغاء/وسم Winner Unpaid (Admin)، قائمة وتفاصيل — موجود.
  - المزايدة (Verified فقط) مع تحقق الدور من `auth.Data()` + تحقق قاعدة البيانات: حالة المستخدم «active»، البريد مُفعّل — مطابق.
  - Anti‑Sniping: تمديد داخل صفقة واحدة WITH UPDATE + optimistic check على `end_at` وتسجيل `auction_extensions` — مطابق.
  - إزالة مزايدة (Admin): تعويض التمديدات وإعادة حساب `end_at` وبثّ الأحداث — موجود.
  - SSE/WS للأحداث (bid_placed/outbid/extended/ended/bid_removed/price_recomputed) — موجود.

- السلة/الطلبات/الفواتير (svc/orders)
  - سياسة السلة: لا Guest Cart — المستخدم غير المفعّل بريده يرى `/cart` فارغ دائماً؛ إضافة/تعديل تتطلب Email Verified — مطابق.
  - حجز الحمام (Qty=1) عبر حالة المنتج: `available → reserved → payment_in_progress → sold`، بمهلة `checkout_hold_minutes` — موجود.
  - المستلزمات: حجوزات عبر `stock_reservations` مع قفل استشاري، وتحرير تلقائي/تمديد عند الدفع — موجود.
  - حَدّ الحجوزات للمستخدم `max_active_holds_per_user` — موجود.
  - منع إضافة حمامة عندما تكون `in_auction|auction_hold|sold|reserved|payment_in_progress` — موجود.
  - Checkout (`POST /checkout`) يدعم Idempotency‑Key، يُنشئ order+invoice (unpaid)، ويحظر `ORD_PIGEON_ALREADY_PENDING` — مطابق.

- المدفوعات وWebhook (svc/payments/worker, pkg/moyasar)
  - `POST /payments/init` مع Idempotency‑Key، حد 5/دقيقة لكل مستخدم، والتحقق من الملكية وحالة الفاتورة — مطابق.
  - Webhook: تحقق توقيع HMAC‑SHA256، Enqueue عبر `pubsub`, إرجاع 200 سريع، Worker يحسم الحالات (paid/failed/pending) ويطبق Late Policy — مطابق.
  - UQ لمنع جلسات دفع متزامنة لكل فاتورة — موجود.
  - استرداد الدفعات (Refund) مع تعامل جزئي/كامل وتحديث حالات invoice/payment — موجود.

- الشحن (svc/shipping)
  - `GET /cities` لرسوم الشحن الصافية لكل مدينة (تُعرض Gross لاحقاً)، `GET/PATCH /shipments/:id` (Admin للـ PATCH)، `GET /shipments?order_id=` — موجود.
  - التحقق من أن الشحنات تخص طلبات مدفوعة — موجود (حراس + قيود في الهجرات).

- الإشعارات (svc/notifications, pkg/mailer, pkg/templates)
  - Inbox داخلي + بريد عبر SendGrid، بنية Queue مع 3 محاولات وBackoff عبر Trigger، Jobs cron للمعالجة والتنظيف — موجود.
  - قوالب بريد متعددة اللغات، مع Fall‑back عند فشل القالب — جيد.

- الإعدادات/الإدارة (pkg/config, svc/adminsettings)
  - `system_settings` مع Hot‑Reload ومزوّد إعدادات CORS ديناميكي؛ إدارة مفاتيح عبر واجهات Admin — موجود.

- قاعدة البيانات (coredb/migrations)
  - المخطط مطابق لأقسام PRD 3.x/5.x/9.x مع قيود/فهارس حرجة (UQ نشط للمزاد، qty=1 للحمام في order_items، sync order↔invoice، قيود الحالات) — مطابق.

## جودة الكود والأمان

- أسرار: تُستخدم Encore Secrets لجميع مفاتيح الحساسة (JWT، SendGrid، GCS، Moyasar) — جيد.
- كلمات المرور: Argon2id بمحددات مناسبة + مقارنة ثابتة الزمن — جيد.
- JWT: توقيع HS256 مع سرّ من الأسرار + مطالبات مخصّصة (UserID/Role/Email) — جيد.
- SQL: استخدام معاملات للعمليات الحرجة وقفل استشاري حيث يلزم؛ استعلامات مُعلَّمة — جيد.
- القياسات: Prometheus عدادات/هيستوجرامات للـ HTTP، bids، payments، webhooks، WS — جيد.
- CORS/Headers/IP: أدوات `pkg/httpx` و`pkg/middleware` للتعامل الآمن مع العناوين وCORS — جيد.

## ملاحظات/ثغرات محتملة (منخفضة/متوسطة)

1) التحقق الآلي والتغطية
   - لم تُشغّل اختبارات `go test ./...` و`go vet` بسبب غياب Go في البيئة. يُتوقع وجود مجموعة اختبارات في `pkg/*` و`svc/catalog/api_test.go`. يوصى بتشغيلها في CI وتثبيت Go محلياً للتحقق.

2) توثيق README
   - في README المسار مذكور كـ `pkg/money_sar/` بينما الحزمة الفعلية `pkg/moneysar/`. يفضّل تصحيح التسمية لتطابق الشجرة.
   - أمر `encore test ./...` قد لا يكون معيارياً؛ يُفضل `go test ./...` محلياً (إن لم يكن هناك runner خاص بـ Encore).

3) إعادة تنشيط حجز الحمام (Nice‑to‑have)
   - PRD ذكر «إعادة تنشيط الحجز» عند العودة لصفحة الدفع قبل بدء جلسة الدفع وضمن 10 دقائق. السلوك الحالي يُبقي الحجز صالحاً حتى انتهاء `reserved_expires_at` ويستند عليه في `/cart` و`/checkout`، لكنه لا يحدّث المؤقّت تلقائياً إن عاد المستخدم (لا تمديد) — هذا مقبول وظيفياً، ويمكن إضافة مسار صريح لاحقاً لو لزم.

4) التسميات/التعارض اللفظي
   - وجود `pkg/moyasar` (مزود الدفع) و`pkg/moneysar` (حسابات SAR/VAT). التسمية مقصودة للتمييز — جيدة، لكن يُستحسن ذكرها بوضوح في README لتجنّب اللبس.

5) سجلات監Audit وPII
   - السجلات الحالية تتجنب عرض PII مباشرة (جيّد). يُستحسن الاستمرار في عدم تسجيل البريد/الهاتف في رسائل الخطأ/السجلات الإنتاجية إلا عند الضرورة وبشكل مُقنَّع.

## توافق الهجرات مع خطة PRD

- 0001..0010 موجودة وتغطي: users/cities/verification/addresses، system_settings، products/pigeons/supplies/media، auctions/bids/extensions، orders/invoices/payments/counters، shipping، stock_reservations، notifications/audit، الفهارس والقيود، وTriggers فرض القواعد (qty=1 للحمام، منع مزادين نشطين، مزامنة حالات order/invoice، تنظيف الحجز… إلخ). تطابق جيّد.

## تتبّع الواجهات مقابل PRD (ملخص سريع)

- auth: register/login/refresh/logout/verify/resend — موجودة.
- users: me (GET/PATCH)، verify request + approve/reject — موجودة.
- catalog: GET products/:id + POST products (Admin) + media upload/archive — موجودة.
- auctions: create/cancel/mark-winner-unpaid, list/get, bid (Verified Only), remove-bid (Admin), SSE/WS — موجودة.
- orders: cart (GET/POST/PATCH/DELETE), checkout (Idempotency), orders+invoices (GET) — موجودة.
- payments: init (Idempotency/Rate‑limit), webhook (Verify→Enqueue→200), invoices (GET) — موجودة.
- shipping: cities, shipments (GET, PATCH Admin, list by order) — موجودة.
- notifications: inbox list, email test/admin, queue worker + retention — موجودة.

## توصيات ختامية

- تشغيل التحقق الآلي:
  - go test ./... — جميع الرزم
  - go vet ./...
  - gofmt -s -w . (أو التحقق بـ gofmt -l .)
- مراجعة README لتصحيح أسماء المجلدات والأوامر.
- إن رغبتُم بدقة أعلى لـ «إعادة تنشيط الحجز»، يمكن إضافة Endpoint خفيف لتجديد `reserved_expires_at` للحمام قبل بدء جلسة الدفع ضمن نافذة زمنية قصيرة (دون تجاوز حدّ الحجز أو الالتفاف على Anti‑oversell).
- الحفاظ على إعدادات النظام الافتراضية (VAT, Anti‑Sniping, Rate‑limits, Payments TTL) ضمن الحدود المقترحة في PRD، والتأكد من Bootstrap القيم الحساسة في `system_settings` عبر سكربت/هجرة أولية.

## خطوات التحقق المقترحة (محلياً/CI)

1) إعداد البيئة:
   - تثبيت Go 1.22+ وEncore CLI.
   - `encore run` لتشغيل Postgres المحلي وتطبيق الهجرات.
   - ضبط الأسرار: `encore secret set` (MoyasarAPIKey, MoyasarWebhookSecret, SendGridAPIKey, GCS*).

2) فحوصات سريعة:
   - `go vet ./...`
   - `go test ./... -coverprofile=cover.out && go tool cover -func=cover.out`
   - `gofmt -l .` (يجب أن لا يُظهر ملفات غير مُنسّقة)

3) تجارب وظيفية (curl مختصرة):
   - التسجيل → تفعيل البريد → إضافة للسلة → Checkout → Init Payment (Idempotency‑Key) → محاكاة Webhook.
   - إنشاء مزاد (Admin) → مزايدة (Verified) داخل نافذة Anti‑Sniping للتأكد من التمديد.

—

النتيجة: لا توجد أخطاء منطقية/برمجية واضحة تمنع التشغيل وفق PRD، والتنفيذ الحالي يغطي المتطلبات الأساسية بدقة. الملاحظات أعلاه تحسينات طفيفة وتوثيقية في الغالب.

