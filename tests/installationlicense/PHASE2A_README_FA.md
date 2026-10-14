# LIC-A9 — آزمون سورس مشترک بدون VPS

**2026-10-14 · NONPRODUCTION / TEST ONLY**

مرجع Authority: PR32 SHA `68cdb6996f85981c36dd20ecb7435ec010351e9b`. مرجع کلاینت TELMIVO: SHA دقیق Commit مورد آزمون در PR47؛ نام شاخه به تنهایی کافی نیست. هیچ داده عملیاتی از سرور لایسنس قدیمی نیاز نیست.

## اجرای کنترل روی دو کلون واقعی

در محیط **موقت و مستقل** که هر دو مخزن خصوصی در آن دریافت شده‌اند، Authority را دقیقاً روی SHA بالا و TELMIVO را روی SHA بررسی‌شده PR47 قرار دهید. هر دو worktree باید پاک باشند. این ابزار قبل از اجرای آزمون، نبودن فایل‌های تغییرکرده یا untracked را نیز بررسی می‌کند.

```sh
python3 -B tests/installationlicense/run_source_preflight.py \
  --authority /absolute/path/TELMIVO-LICENSE-AUTHORITY \
  --product /absolute/path/TELMIVO \
  --expected-product-sha <EXACT_40_CHARACTER_TELMIVO_COMMIT_SHA>
```

این دستور بدون راه‌اندازی دیتابیس، ابتدا سه گروه آزمون Python و کنترل استاتیک قرارداد M2M را اجرا می‌کند، سپس نسخه Go را بررسی می‌کند و در صورت واجدشرایط‌بودن، `go test -count=1 ./internal/platform/installationlicense` را اجرا می‌کند.

**پیش‌نیاز:** فایل `go.mod` کلاینت Go **1.27.0** اعلام می‌کند؛ Go 1.25 یا Go 1.23 کافی نیست. اجرای `go test` ممکن است به ماژول‌های cache نشده نیاز داشته باشد؛ این ابزار اجازه اجرا روی Production، دانلود مخفی سورس خصوصی یا استفاده از اعتبارنامه واقعی نمی‌دهد.

## مدرک آزمون کنترل‌کننده

- دو فایل `run_source_preflight.py` و `test_source_preflight.py` با bytes برابر با GitHub روی محیط موقت بازسازی شدند و Git blob SHA1 آنها با blob ثبت‌شده روی GitHub مطابقت داده شد.
- **۱۲ آزمون Python از ۱۲ آزمون PASS**، شامل تشخیص SHA غلط، checkout مشترک، تغییر ثبت‌نشده، خطای child process و آزمون عملی `git init/commit/status` روی یک مخزن کاملاً موقت.
- دستور: `python3 -B -m unittest discover -s tests/installationlicense -p 'test_source_preflight.py' -v` (در این مرحله همان فایل‌های byte-verified در محیط مستقل اجرا شدند).
- این نتیجه **اجرای runner با mockهای کنترل‌شده** است؛ test runner به صورت سرتاسری از دو کلون کامل، با Go 1.27، اجرا نشده است.

## گیت باقی‌مانده

آزمون Go/PostgreSQL 16 مشترکِ `Authority + TELMIVO` از سورس دقیق هنوز **NOT_RUN** است. `authority_e2e_test.go.template` قبلاً برای نسخه قدیمی‌تر Authority نوشته شده و ممکن است نیازمند اصلاحات صرفاً آزمایشی باشد. تا اجرای واقعی، وضعیت `PRODUCT_E2E_PASS` یا `LICENSE_INSTALLER_READY` صادر نشود.

هیچ VPS، توکن Telegram، کلید خصوصی Production یا دیتابیس عملیاتی در این مرحله مورد استفاده قرار نمی‌گیرد.
