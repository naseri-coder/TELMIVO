# TELMIVO — TeraBox Official API Preflight (FA)

**وضعیت: مستندسازی و آمادگی فنی — NONPRODUCTION ONLY — هیچ اتصال واقعی برقرار نشده است.**

**تاریخ تحقیق رسمی:** 2026-10-10. **مخزن:** `naseri-coder/TELMIVO`.

## ۱. حساب رایگان با API برابر نیست

وب‌سایت رسمی: https://www.terabox.com/

مستندات رسمی Open Platform:
https://www.terabox.com/integrations/docs?lang=en
https://www.terabox.com/integrations/docs/chinese

TeraBox روش‌های ثبت‌نام با ایمیل/گوگل را معرفی می‌کند؛ اما در مستندات رسمی اتصال برنامه‌ها تصریح شده است که `client_id`، `client_secret` و `private_secret` باید **جداگانه درخواست شوند**. دریافت این مقادیر برای هر حساب رایگان تضمین نشده است.

### نکته حیاتی درباره دامنه

`terabox.com` دامنه موردنظر این پروژه است. سایت‌های مشابه‌الاسم یا دامنه‌های با املای متفاوت را مبنای دریافت API، ورود حساب یا نصب اسکریپت قرار ندهید.

## ۲. مرحله اپراتور — بدون افشای اطلاعات

۱. در وب‌سایت رسمی ثبت‌نام کنید؛ طرح Premium، اشتراک پولی یا دوره آزمایشی نیاز نیست.
۲. ایمیل را تأیید و روش امن بازیابی حساب را تنظیم کنید.
۳. **سهمیه واقعی نمایش‌داده‌شده پس از ورود** را ثبت کنید. تبلیغ ۱۰۲۴GB را با ظرفیت تضمین‌شده حساب اشتباه نگیرید.
۴. وجود محدودیت حجم هر فایل، دانلود، ترافیک، تعداد فایل، سرعت، محدودیت فعالیت حساب و شرایط حذف/انقضای سهمیه رایگان را در حساب خود کنترل کنید.
۵. برای Open Platform و امکان اتوماسیون از پشتیبانی رسمی درخواست شفافیت کنید؛ مشخصاً دسترسی حساب رایگان به API، نرخ درخواست‌ها، انتقال دانلود و رفرش توکن را سؤال کنید.
۶. **در گفتگو یا GitHub هیچ ایمیل خصوصی، رمز، کوکی مرورگر، refresh/access token، `client_secret`، `private_secret` یا کلید بازیابی قرار ندهید.**

پس از ثبت‌نام فقط اعلام کنید: حساب ساخته شد (بله/خیر)، ظرفیت واقعی (عدد بدون شناسه شخصی)، برنامه رایگان است (بله/خیر)، API رسمی صادر شده (بله/خیر/درخواست نشده).

## ۳. ریسک‌های فنی API

طبق مستندات رسمی:

- Authorization-code / Device-code همراه با تأیید کاربر.
- `access_token` با طول عمر مستند حدود **۲ روز** و `refresh_token` حدود **۳۰ روز**. رفتار جزئی چرخه تمدید، مصرف یک‌بار و محدودیت‌های اپراتوری باید در حساب آزمایشی احراز شود.
- پس از دریافت توکن، برای تشخیص `api_domain` و `upload_domain` مختص همان حساب، باید جریان رسمی `/oauth/tokeninfo` طی شود.
- از روی مستندات موجود، **S3 compatibility، Object Lock غیرقابل‌حذف، exact immutable versionId، CAS witness و استقلال نگهداری Pin اثبات نشده‌اند**.
- آپلود موفق یا برابر بودن SHA256 فایل دریافتی، تأیید بازیابی PostgreSQL، امضای رسید یا امنیت ضدبازگشت حالت License Authority نیست.
- به‌روزرسانی API و سقف‌ها می‌توانند تغییر کنند. برای ادعای Production به آزمایش واقعی با اعتبارنامه محدود و مجوز جداگانه نیاز داریم.
- شرایط خدمات TeraBox اجازه تغییر میزان Free Storage Capacity و محدودیت سرویس را می‌دهد: https://www.terabox.com/terms/duty ؛ سهمیه یک‌ترابایت و رایگان‌بودن دائمی قابلیت API تضمین قراردادی نیست.

## ۴. قرارداد امنیتی برای آزمایش پس از دریافت API

تست بعدی **فقط با مجوز مستقل** و در محیط ایزوله:

- ابتدا یک فایل کاملاً ساختگی و رمزنگاری‌شده با `age` (کلید TEST) تولید شود.
- با احراز هویت رسمی OAuth (بدون کوکی‌های استخراجی یا ربات مرورگری غیررسمی)، از یک هویت کم‌دسترسی استفاده شود؛ رمز و توکن فقط در مخزن اسرار محافظت‌شده، نه در GitHub، لاگ یا چت.
- آپلود از مسیر رسمی، شناسایی دقیق شیء در API و ثبت طول/هش.
- دریافت مستقل تمام بایت‌ها؛ تطبیق SHA256 + طول؛ رمزگشایی با TEST identity.
- رفتار قطع اتصال، تکرار درخواست، محدودیت حساب، تمدید توکن، نام تکراری و حذف فایل در محیط TEST بررسی شود.
- اگر امکان شناسایی exact version / قفل غیرقابل‌حذف / بازیابی مستقل ثابت نشود، **TeraBox فقط نسخه ثانویه اختیاریِ encrypted copy** است و هرگز شرط اصلی بازیابی License Authority را برآورده نمی‌کند.
- هیچ داده واقعی کاربر یا دیتابیس Production تا تأیید رسمی E2E آپلود نشود.
- پوشه‌های پیشنهادی فقط برای نظم‌دهی هستند: `TELMIVO-MAIN-DATABASE` و `TELMIVO-LICENSE-AUTHORITY`. پوشه‌ها جداسازی امنیتی واقعی حساب و کلید نیستند.

## ۵. سیاست هدف؛ هنوز غیرفعال

| سیستم | فاصله هدف | محدوده داده | وضعیت |
|---|---:|---|---|
| TELMIVO Main | هر ۱۰ دقیقه (قابل بازبینی ۵/۱۵ دقیقه) | فقط PostgreSQL تولیدی `telmivo` | نه فعال، نه Production-Qualified |
| License Authority | روزانه پس از ایجاد دیتابیس عملیاتی | DB + شواهد معتبر بازیابی؛ لاگ تغییر امنیتی خارج از فواصل snapshot | نه فعال، نه Production-Qualified |

روابط مالی، لایسنس و امنیت فقط با یک بکاپ روزانه قابل بازگردانی معتبر نیستند: پس از ابطال/بازنشستگی/نسل جدید، برگشت به snapshot قدیمی بدون تاریخچه قابل‌بازتولید ممنوع است. در ابهام، Recovery Hold.

## ۶. پیام پیشنهادی برای سؤال از پشتیبانی

Subject: Official Open Platform API access for encrypted automated backups

Hello TeraBox Support,

I plan to use a free TeraBox account only for encrypted, non-sensitive test backup files first, and would like to integrate through your official Open Platform APIs. Could you clarify (1) whether personal/free accounts are eligible to apply for client_id/client_secret/private_secret, (2) how to apply, (3) any fees, rate limits, upload/download size and bandwidth limits, (4) unattended refresh-token restrictions, and (5) whether immutable file versions, write-once retention or object-lock APIs are supported?

Please share the official application process and relevant public documentation. Thank you.

**پیام را اپراتور از حساب خودش ارسال می‌کند؛ هیچ توکن یا شناسه خصوصی لازم نیست.**

## ۷. فهرست توقف

`TERABOX_ACCOUNT_CREATED=UNCONFIRMED`

`TERABOX_OFFICIAL_API_GRANTED=FALSE`

`TERABOX_REAL_UPLOAD_TEST=NOT_RUN`

`TERABOX_INDEPENDENT_DOWNLOAD=NOT_RUN`

`TERABOX_OBJECT_IMMUTABILITY_QUALIFIED=FALSE`

`TERABOX_LICENSE_DR_QUALIFIED=FALSE`

`MAIN_DB_BACKUP_TIMER_ENABLED=FALSE`

`LICENSE_DB_BACKUP_TIMER_ENABLED=FALSE`

No account creation, secrets, provider requests, Production backup, deployment, merge or timer activation is authorized by this document.
