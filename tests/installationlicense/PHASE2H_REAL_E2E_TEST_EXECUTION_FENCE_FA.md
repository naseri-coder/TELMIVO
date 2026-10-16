# TELMIVO LIC-A9 Phase 2H — الزام اثبات اجرای واقعی شش آزمون E2E

**وضعیت: تکمیل اصلاح مسیر آزمون — آزمون واقعی PostgreSQL هنوز NOT_RUN است.**

## اشکال قطعیِ کشف‌شده

دستور `go test -run '^TestE2E[1-6]$'` حتی زمانی که **هیچ آزمونی** با این نام‌ها وجود نداشته باشد، با کد خروج `0` و عبارت `PASS` پایان می‌یابد. این رفتار با خود Go 1.23.2 در یک پروژه موقت مستقل از هر سرور/دیتابیس بازتولید شد؛ خروجی `testing: warning: no tests to run` بود. پیش از این اصلاح، runner ممکن بود `LIC_A9_JOINT_DOCKER_GO127_POSTGRES16_E2E_PASS` را به اشتباه اعلام کند.

## اصلاح دقیق و محدود

در `tests/installationlicense/run_joint_docker_only_e2e.py`:

1. پس از کامپایل و **پیش از ساخت کانتینر PostgreSQL**، `go test -list '^TestE2E[1-6]$' ./authority` اجرا و نام دقیق شش آزمون کنترل می‌شود. نام تکراری/ناقص/اضافی باعث `NOT_PASS` است.
2. آزمون یکپارچه با `go test -count=1 -json -timeout 8m -run '^TestE2E[1-6]$' ./authority` اجرا می‌شود. برای `TestE2E1` تا `TestE2E6` دقیقاً یک رویداد `run` و یک رویداد `pass`، همراه موفقیت package، الزامی است.
3. شکست، skip، گزارش JSON ناقص، package-only PASS، کمبود رویدادها یا تکرار آن‌ها باعث `NOT_PASS` است. شرط توقف موفق کانتینر موقت از Phase 2G حفظ شده است.
4. هیچ فایل Go، سیاست لایسنس، credential، migration یا دیتابیس عملیاتی تغییر نمی‌کند.

## شواهد اجرای واقعی

- Go روی پروژه مصنوعیِ موقت: ۰ آزمونِ انتخاب‌شده، **exit 0**؛ parser آن را صریحاً رد می‌کند.
- Go روی پروژه مصنوعی با دقیقاً ۶ آزمون: `go test -list` و `go test -json` اجرا شدند و هر شش نام و رویداد موفقیت مستقل تأیید شدند.
- تست‌های mock مسیر Docker اطمینان می‌دهند کشف صفر آزمون قبل از ساخت دیتابیس متوقف می‌شود و package-only PASS پس از اجرا باز هم `NOT_PASS` است و پاک‌سازی انجام می‌شود.
- کل suite محلی با فایل‌های متناظر با commit: **36/36 PASS**، دستور:
  `python3 -B -m unittest discover -s . -p 'test_*.py' -v`
- Git blob SHA نسخهٔ اجراشده:
  - `run_joint_docker_only_e2e.py`: `210aed7da3ef7ab2fadfde56068b4d9d99e610b0`
  - `test_joint_docker_only_e2e.py`: `b18ad417f5affed35c1f2826841b2637702b22de`

**حدود اعتبار:** این اصلاح از اعلام PASS کاذب جلوگیری می‌کند، اما آزمایش مشترک دو مخزن اصلی با Docker/Go1.27/PostgreSQL16 در محیط فعلی هنوز اجرا نشده است. سرور عملیاتی Telmivo و شاخه main دست نخورده‌اند، GitHub Actions اجرا نشده است.

**Verdict:** `GO_ZERO_TEST_FALSE_PASS_FENCED`; `PHASE2H_LOCAL_36_OF_36_PASS`; `JOINT_AUTHORITY_PG16_E2E_NOT_RUN`.
