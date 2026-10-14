# LIC-A9 Phase 2E — رفع مانع دوم ماژول Go در fixture مشترک

**2026-10-14 — فقط محیط موقت، بدون GitHub Actions، بدون سرور یا دیتابیس عملیاتی.**

## مشکل قطعی و آزمایش بازتولید

`prepare_authority_fixture.py` محتوای `git archive` کامل Authority را در `<fixture>/authority` باز می‌کرد؛ این آرشیو شامل `go.mod` و `go.sum` مخزن Authority است. خودش نیز `<fixture>/go.mod` والد را می‌ساخت. بنابراین `go test -run '^$' ./authority` از پوشهٔ والد، بستهٔ Authority را داخل یک ماژول **nested** مستقل می‌دید و پیش از اتصال به PostgreSQL خطای زیر رخ می‌داد:

```text
main module (scratch.test/joint) does not contain package scratch.test/joint/authority
```

این رفتار با Git و Go واقعی در یک آزمایش مستقل بازتولید شد. برداشتن `go.mod` داخلی در **کپی موقت فقط** مسیر `go list ./authority` را اصلاح کرد.

## اصلاح

- بعد از بازکردن آرشیو در **پوشهٔ موقت**، پیش از بازنویسی importهای آزمایشی، `go.mod` درون `authority/` بررسی می‌شود تا نام ماژول اصلی دقیقاً `github.com/naseri-coder/TELMIVO-LICENSE-AUTHORITY` باشد.
- فقط `go.mod` و `go.sum` مربوط به کپی موقت حذف می‌شوند، نه فایل‌های checkout خصوصی، نه سورس یا manifest عملیاتی.
- `run_joint_postgres_e2e.py` دیگر تغییر تأییدشدهٔ preparer را خودکار رد نمی‌کند، بلکه **Git blob SHA دقیق** `be27fbdf5871ee54e35ef9302d2749a67b1725e9` را الزامی می‌کند. سایر محدودیت‌های SHA دو مخزن و سورس اجرایی ثابت می‌ماند.

## شواهد اجرا و آزمون

فایل‌های آزمایشی از GitHub به محیط موقت منتقل شدند و `git hash-object` محلی با GitHub برابر بود:

- `prepare_authority_fixture.py`: `be27fbdf5871ee54e35ef9302d2749a67b1725e9`
- `test_nested_fixture_module.py`: `eac3289a674ebf0c60eba4e5ef0be5f7cf9bae9e`
- `run_joint_postgres_e2e.py`: `734847fd06ec2d50552c9677b593aea8e9b99cee`
- `test_joint_runner.py`: `064fca8060c3be5b5ab6f1c7c16d6385529a9c2f`

```sh
python3 -B -m unittest -v test_joint_runner.py test_nested_fixture_module.py
```

**۲۱ از ۲۱ آزمون PASS**. از این تعداد پنج آزمون جدید با مخزن Git واقعیِ موقت، اجرای واقعی `git archive` و `tar` و `go list ./authority` و `go test -count=1 -run '^$' ./authority` (Go 1.23.2) هستند. آزمون‌های منفی انتساب SHA نامعتبر، ماژول غیرمنتظره و مقصد تکراری را هم رد کردند. ۱۶ آزمون قبلیِ ابزار offline همچنان PASS هستند.

این یک **آزمون موضعیِ ساخت fixture با سورس مصنوعی** است، نه کامپایل کامل دو مخزن و نه اجرای شش تست `TestE2E1..6` در PostgreSQL.

## گیت باز

آزمون مشترک واقعی با دو مخزن خصوصی مرج‌شده، Go 1.27 و PostgreSQL 16 محلی همچنان `NOT_RUN` است. محیط فعلی Docker، Go 1.27 و clone کامل خصوصی ندارد؛ اجازه استفاده از سرورهای عملیاتی برای آزمون داده نشده است.

**Verdict:** `NESTED_GO_MODULE_FIXTURE_BLOCKER_RESOLVED_LOCALLY`, `OFFLINE_FIXTURE_AND_RUNNER_TESTS_21_OF_21_PASS`, `CROSSREPO_GO127_POSTGRES16_E2E_NOT_RUN`.
