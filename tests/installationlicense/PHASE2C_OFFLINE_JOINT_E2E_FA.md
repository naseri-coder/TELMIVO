# LIC-A9 — مسیر مستقل آزمون مشترک Go 1.27 و PostgreSQL 16 (بدون GitHub Actions)

**وضعیت: PREPARED + OFFLINE SAFETY-TESTED. آزمون دیتابیس در این مرحله اجرا نشده است.**

## مرجع سورس

- Authority، نسخه مرج‌شده در شاخه توسعه PR32: `939d14800c602dd9c31ff23fdf6e0688a440d4c2`.
- TELMIVO، نسخه runtime مرج‌شده PR47: `bc417045ca5e1b91a2377aba6dece348699f7bf4`.
- نسخه آزمایشگاهی `prepare_authority_fixture.py` همچنان سورس اجرای Authority را از SHA اصلی PR32 (`68cdb6996f85981c36dd20ecb7435ec010351e9b`) آرشیو می‌کند. مرج بعدی Authority فقط آزمون و مستندات اضافه کرد، نه منطق runtime. تست کامل بر مبنای نسخه اجرایی PR32 است، نه ادعای اجرای source tree کامل جدید.

## اجرا روی کامپیوتر توسعه موقت و مستقل

**روی سرور اصلی TELMIVO اجرا نکنید.** به Docker Engine با context محلی Unix socket، تصویر PostgreSQL 16، Go حداقل 1.27، Python و دو checkout خصوصیِ پاک نیاز دارد. این ابزار VPS نمی‌سازد و هیچ GitHub Actions را اجرا نمی‌کند.

```sh
python3 -B tests/installationlicense/run_joint_postgres_e2e.py \
  --authority /path/to/TELMIVO-LICENSE-AUTHORITY \
  --product /path/to/TELMIVO \
  --acknowledge-disposable-host
```

ابزار پیش از شروع دیتابیس، کنترل می‌کند که checkout مرجع Authority دقیقاً SHA فوق باشد و checkout TELMIVO از SHA مبنا منشعب شده و فایل‌های `go.mod`، `go.sum`، `internal/` و fixture اصلی از آن زمان تغییر نکرده باشند. تغییر فقط در ابزارهای test و documentation مجاز است. Worktreeهای دارای تغییرات ثبت‌نشده رد می‌شوند.

سپس یک کپی موقت fixture را می‌سازد و ابتدا `go test -run '^$' ./authority` را برای تشخیص خطای کامپایل اجرا می‌کند؛ در صورت خطا **هیچ Docker PostgreSQL ساخته نمی‌شود**. پس از موفقیت کامپایل، یک کانتینر تست با RAM/CPU محدود، پورت تصادفی `127.0.0.1` و بانک داده ساختگی می‌سازد. آدرس دیتابیس فقط در حافظه فرایند آزمون تولید می‌شود و `LIC_A8_TEST_DATABASE_URL` برای شش آزمون `TestE2E1` تا `TestE2E6` تنظیم می‌شود. در انتها کانتینر متوقف شده و fixture موقت حذف می‌شود. اتصال Docker راه‌دور و متغیرهای شناخته‌شده محیط Production رد می‌شوند.

## نتایج واقعی موجود

در محیط مستقل فعلی فقط Go 1.23.2 موجود است؛ Docker، PostgreSQL، checkout کامل مخازن خصوصی و اتصال مستقیم به اینترنت وجود ندارند. بنابراین هیچ E2E واقعی با PostgreSQL اجرا نشده است.

ابزار `run_joint_postgres_e2e.py` و آزمون `test_joint_runner.py` روی همین محیط موقت اجرا و با Git blob SHA محتوای ثبت‌شده در GitHub یکسان شدند:

```sh
python3 -B -m unittest discover -s tests/installationlicense -p test_joint_runner.py -v
```

**نتیجه: ۱۳ آزمون از ۱۳ آزمون PASS**، شامل بررسی نسخه Go، پورت محدود به loopback، حذف متغیرهای حساس از محیط Go، رد Docker راه‌دور، جلوگیری از سورس آلوده، بررسی واقعی Git روی مخزن موقت، شبیه‌سازی تقدم کامپایل نسبت به Docker و فراخوانی cleanup پس از خطای E2E.

**وضعیت نهایی:** `OFFLINE_RUNNER_PRECHECK_13_OF_13_PASS`؛ `JOINT_GO127_POSTGRES16_E2E_NOT_RUN`؛ `NO_ACTIONS_NO_MAIN_SERVER_NO_DEPLOY`.

پس از دسترسی به یک محیط **واقعاً موقت و جدا از سرور اصلی**، باید همین ابزار را اجرا کرد و بر اساس خروجی واقعی درباره ادغام PR32 به main تصمیم گرفت.


## مرحلهٔ بعدی — اصلاح مرجع دقیق Archive در آزمون مشترک

در بازبینی پس از PR48 یک شکاف در انتساب نسخه کشف شد:

- `run_joint_postgres_e2e.py` checkout مرج‌شدهٔ Authority را روی `939d14800c602dd9c31ff23fdf6e0688a440d4c2` پین می‌کرد.
- `prepare_authority_fixture.py` همچنان کد آزمون E2E را با `git archive` از `68cdb6996f85981c36dd20ecb7435ec010351e9b` استخراج می‌کرد.
- با `compare_commits` GitHub مشخص شد تنها تغییر بین دو Commit، افزودن گزارش `docs/development/LIC_A9_PHASE2B_AUTHORITY_PG16_CI_BLOCKER_20261014.md` و آزمون `internal/domain/phase2b_safety_test.go` است؛ هیچ فایل runtime یا migration تغییر نکرده است.

**اصلاح اجراشده:** قبل از هر کامپایل یا ساخت کانتینر، تابع `validate_authority_archive_provenance` با Git واقعی بررسی می‌کند که SHA آرشیو جدّ SHA مرج‌شده باشد و فهرست دقیق تغییرات بین آن‌ها **فقط همین دو فایل غیرعملیاتی** باشد. اضافه‌شدن هر فایل دیگر (حتی فایل تست جدید) یا تغییر کد اجرایی باعث خطای `NOT_PASS` می‌شود. در این وضعیت، برابر بودن سورس runtimeِ نسخه آرشیوشده و نسخه مرج‌شده یک فرض تأییدنشده نیست، بلکه شرط اجرای ابزار است.

**نتیجهٔ محلی:** `16/16` آزمون Python شامل Git واقعی موقت، تشخیص تغییر runtime، رد تغییر غیرمنتظره و جلوگیری از Go/Docker در وضعیت ناسازگار موفق شدند. SHA Git blob فایل‌های محلی اجراشده با GitHub تطبیق داده شد.

**گیت باز:** همچنان اجرای `go test` مشترک با Go 1.27 و PostgreSQL 16 انجام نشده است. هیچ تغییر Production، GitHub Actions یا سرور اصلی صورت نگرفته است.
