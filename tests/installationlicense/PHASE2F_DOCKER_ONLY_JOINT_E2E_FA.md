# LIC-A9 — Phase 2F: Docker-only Joint E2E (بدون Go میزبان و بدون GitHub Actions)

**وضعیت:** `PREPARED / OFFLINE TESTED`، **نه** `POSTGRESQL_E2E_PASS`.

## هدف و نتیجه

در Phase 2E مانع nested `go.mod` در fixture برطرف شد، اما ابزار E2E هنوز به نصب جداگانه Go 1.27 روی میزبان احتیاج داشت. این مرحله `tests/installationlicense/run_joint_docker_only_e2e.py` را اضافه می‌کند:

- اجرای واقعی Go با image رسمی `golang:1.27.0-bookworm`؛ بدون نصب Go روی میزبان؛ منبع نام tag: https://hub.docker.com/_/golang/tags?name=bookworm
- PostgreSQL 16 در کانتینر کاملاً موقت با `--network none`، **بدون `-p` یا هر پورت منتشرشده روی میزبان**.
- پیش از ایجاد دیتابیس، Go به صورت compile-only با شبکهٔ Docker bridge فقط برای دریافت وابستگی‌های لازم اجرا می‌شود. cache در پوشهٔ موقت قرار دارد.
- پس از بالا آمدن دیتابیس، کانتینر Go صرفاً با `--network container:<name-of-disposable-db>` به همان namespace متصل می‌شود؛ `GOPROXY=off` و `GOSUMDB=off` روی مرحله آزمون E2E اعمال می‌شوند.
- product checkout به صورت `readonly` mount می‌شود؛ فقط fixture و cache موقت قابلیت نوشتن دارند؛ هیچ Docker socket/secret میزبان داخل کانتینر Go mount نمی‌شود.
- کنترل‌های قبلی برای SHAهای مرج‌شدهٔ Authority/TELMIVO، تمیزی Git، تفاوت ثابت نسخهٔ آرشیوشدهٔ Authority، سورس اجرایی و SHA ابزار preparer بدون کاهش حساسیت استفاده می‌شوند.
- فقط Docker context محلی Unix socket قابل قبول است. `DOCKER_HOST` راه دور و متغیرهای محیطی شناخته‌شدهٔ عملیاتی باعث `BLOCKED` می‌شوند. `docker stop` در پایان یا شکست فراخوانی می‌شود.

## شرایط اجرای واقعی

فقط **ماشین توسعهٔ موقتِ مستقل، خارج از VPS اصلی TELMIVO** و دارای Python 3.10+، Git، Docker Engine و دسترسی به دو checkout خصوصی و پاک مورد نیاز است. Go 1.27، PostgreSQL یا Docker Compose روی میزبان لازم نیست؛ imageها توسط Docker روی همان ماشین دریافت می‌شوند. این مرحله هیچ سرور/اکانت جدیدی provision نکرده است.

```sh
python3 -B tests/installationlicense/run_joint_docker_only_e2e.py \
  --authority /absolute/path/TELMIVO-LICENSE-AUTHORITY \
  --product /absolute/path/TELMIVO \
  --acknowledge-disposable-host
```

- Checkout Authority باید دقیقاً `939d14800c602dd9c31ff23fdf6e0688a440d4c2` باشد.
- Checkout TELMIVO باید از `bc417045ca5e1b91a2377aba6dece348699f7bf4` منشعب شده، runtime آن تغییر نکرده و fixture preparer دارای blob SHA تأییدشده باشد.
- اجرای آزمون‌های E2E فقط روی داده‌های ساختگی و بانک موقت انجام می‌شود؛ بدون استفاده از production state/keys.
- نتیجهٔ موفق تنها با خروجی واقعی `LIC_A9_JOINT_DOCKER_GO127_POSTGRES16_E2E_PASS` از ابزار پذیرفته می‌شود. موفقیت آزمون mocks، معادل این نتیجه نیست.

## شواهد عملی در این نوبت

در محیط مستقل فعلی Go میزبان `1.23.2` است، Docker وجود ندارد و دریافت مستقیم GitHub private clones ممکن نیست. بنابراین **PostgreSQL16 E2E اجرا نشده**.

آزمون‌های موجود `21/21` مجدداً موفق شدند. ۹ تست مستقل جدید نیز روی مسیر Docker-only از طریق mock کنترل کردند: image دقیق Go1.27، mount-only read-only، منع شبکهٔ خارجی E2E، منع هر host-published port، کنترل source قبل از Docker، ترتیب compile-before-DB، توقف دیتابیس حتی در failure و رد نسخه اشتباه Go. مجموع:

```text
python3 -B -m unittest discover -s . -p 'test_*.py' -v
Ran 30 tests in 0.406s
OK
```

فایل‌های محلی اجراشده با blobهای GitHub تطبیق داده شدند:
- `run_joint_docker_only_e2e.py`: `37509276c4c2a8413f5435e0b6180bf03d549676`
- `test_joint_docker_only_e2e.py`: `1c4c78fcfdaa417402f10414f91ee5419daa2901`

**حکم:** `PHASE2F_DOCKER_ONLY_RUNNER_30_OF_30_LOCAL_TESTS_PASS`؛ `JOINT_POSTGRESQL16_REAL_E2E_NOT_RUN`؛ `NO_GITHUB_ACTIONS`؛ `NO_PRODUCTION_MUTATION`.
