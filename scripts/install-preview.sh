#!/usr/bin/env bash
# TELMIVO isolated preview installer. Ubuntu 24.04, amd64/arm64.
# Run: bash telmivo-install.sh
set -Eeuo pipefail
umask 077
readonly REPO='https://github.com/naseri-coder/TELMIVO.git'
readonly SOURCE_SHA='71fb2188512d7d4861abc802b635d4119d7d58fc'
readonly INSTALL_DIR="${HOME}/telmivo-test"
readonly PROJECT='telmivo-preview'
AUTH_TMP=''
SOURCE_TMP=''
cleanup() {
  if [[ -n "$AUTH_TMP" ]]; then rm -rf -- "$AUTH_TMP"; fi
  if [[ -n "$SOURCE_TMP" ]]; then rm -rf -- "$SOURCE_TMP"; fi
}
trap cleanup EXIT
trap 'printf "\nنصب متوقف شد؛ هیچ volume حذف نشد. دستور ناموفق در خط %s\n" "$LINENO" >&2' ERR
say() { printf '\n\033[1;36m%s\033[0m\n' "$*"; }
die() { printf '\nخطا: %s\n' "$*" >&2; exit 1; }
priv() { if [[ $EUID == 0 ]]; then "$@"; else sudo "$@"; fi; }
compose() { priv docker compose --project-name "$PROJECT" --project-directory "$INSTALL_DIR" -f "$INSTALL_DIR/docker-compose.yml" -f "$INSTALL_DIR/.telmivo-preview/compose.yml" "$@"; }
main() {
  [[ $# == 0 ]] || die 'این نسخه فقط نصب محیط آزمایشی را انجام می‌دهد؛ بدون آرگومان اجرا کنید.'
  [[ -f /etc/os-release ]] || die 'Ubuntu 24.04 لازم است.'
  # OS metadata is system-owned, not project-supplied configuration.
  . /etc/os-release
  [[ ${ID:-} == ubuntu && ${VERSION_ID:-} == 24.04 ]] || die 'این نصب‌کننده برای Ubuntu 24.04 ساخته شده است.'
  [[ $(uname -m) == x86_64 || $(uname -m) == aarch64 ]] || die 'معماری پشتیبانی نمی‌شود.'
  if [[ $EUID != 0 ]]; then command -v sudo >/dev/null || die 'sudo لازم است.'; sudo -v; fi
  command -v flock >/dev/null || die 'flock لازم است.'
  exec 9>"${HOME}/.telmivo-test-install.lock"
  flock -n 9 || die 'نصب دیگری در حال اجراست.'
  say 'TELMIVO • Naseri coder • نصب آزمایشی خصوصی'
  say '۱/۶ — آماده‌سازی ابزارها؛ بدون حذف قفل apt یا ارتقای کل سیستم'
  priv apt-get -o DPkg::Lock::Timeout=300 update
  priv apt-get -o DPkg::Lock::Timeout=300 install -y ca-certificates curl git gh python3
  if ! command -v docker >/dev/null; then
    priv apt-get -o DPkg::Lock::Timeout=300 install -y docker.io docker-compose-v2
  elif ! priv docker compose version >/dev/null 2>&1; then
    die 'Docker موجود است ولی Compose v2 ندارد؛ برای جلوگیری از تعارض بسته‌ها خودکار جایگزین نمی‌شود.'
  fi
  priv systemctl start docker
  priv docker info >/dev/null
  priv docker compose version >/dev/null
  say '۲/۶ — دریافت checkpoint ثابت؛ ورود GitHub فقط در صورت نیاز'
  if [[ ! -e "$INSTALL_DIR" ]]; then
    if ! gh auth status --hostname github.com >/dev/null 2>&1; then
      [[ -t 0 ]] || die 'ورود GitHub به ترمینال تعاملی نیاز دارد.'
      AUTH_TMP=$(mktemp -d)
      export GH_CONFIG_DIR="$AUTH_TMP"
      say 'کد ورود مرورگر را در گوشی تأیید کن؛ رمز یا token در چت لازم نیست.'
      gh auth login --hostname github.com --git-protocol https --web
    fi
    SOURCE_TMP=$(mktemp -d "${HOME}/.telmivo-source.XXXXXX")
    git -c credential.helper= -c 'credential.helper=!gh auth git-credential' clone --no-checkout "$REPO" "$SOURCE_TMP"
    git -C "$SOURCE_TMP" checkout --detach "$SOURCE_SHA"
    mv -T -- "$SOURCE_TMP" "$INSTALL_DIR"
    SOURCE_TMP=''
  else
    [[ -d "$INSTALL_DIR/.git" ]] || die 'مسیر نصب از قبل وجود دارد و مخزن نیست؛ چیزی بازنویسی نمی‌شود.'
    [[ $(git -C "$INSTALL_DIR" remote get-url origin) == "$REPO" ]] || die 'هویت مخزن موجود متفاوت است.'
    [[ $(git -C "$INSTALL_DIR" rev-parse HEAD) == "$SOURCE_SHA" ]] || die 'HEAD موجود متفاوت است؛ reset یا update خودکار انجام نمی‌شود.'
    [[ -z $(git -C "$INSTALL_DIR" status --porcelain --untracked-files=no) ]] || die 'فایل tracked تغییر کرده است؛ نصب متوقف شد.'
  fi
  [[ $(git -C "$INSTALL_DIR" rev-parse HEAD) == "$SOURCE_SHA" ]] || die 'checkpoint نصب تأیید نشد.'
  # GitHub credentials from a new login are temporary and removed at EXIT.
  # Reject a Compose project of the same name owned by another checkout.
  while IFS= read -r container; do
    [[ -n "$container" ]] || continue
    owner=$(priv docker inspect --format '{{ index .Config.Labels "com.docker.compose.project.working_dir" }}' "$container")
    [[ "$owner" == "$INSTALL_DIR" ]] || die 'پروژه Compose هم‌نام متعلق به مسیر دیگری است.'
  done < <(priv docker ps -aq --filter "label=com.docker.compose.project=$PROJECT")
  say '۳/۶ — تنظیمات synthetic و منابع محلی جدا'
  mkdir -p "$INSTALL_DIR/.telmivo-preview"
  if [[ ! -e "$INSTALL_DIR/.env" ]]; then
    cp "$INSTALL_DIR/.env.example" "$INSTALL_DIR/.env"
    python3 - "$INSTALL_DIR/.env" <<'PY'
from pathlib import Path
import sys
p=Path(sys.argv[1]);s=p.read_text()
s=s.replace('BOT_TOKEN=\n','BOT_TOKEN=synthetic-signing-only\n',1)
s=s.replace('ADMIN_TELEGRAM_IDS=\n','ADMIN_TELEGRAM_IDS=94000001\n',1)
p.write_text(s)
PY
  fi
  chmod 600 "$INSTALL_DIR/.env"
  python3 - "$INSTALL_DIR/.env" "$INSTALL_DIR/.env.example" <<'PY'
from pathlib import Path
import sys

def env(p):
 result={}
 for line in Path(p).read_text().splitlines():
  if not line.strip() or line.lstrip().startswith('#'):continue
  k,v=line.split('=',1)
  if k in result:raise SystemExit('Duplicate environment key; refused')
  result[k]=v
 return result
expected=env(sys.argv[2]); expected['BOT_TOKEN']='synthetic-signing-only'; expected['ADMIN_TELEGRAM_IDS']='94000001'
if env(sys.argv[1]) != expected:raise SystemExit('Environment differs from isolated preview configuration; refused')
PY
  (cd "$INSTALL_DIR" && sh scripts/dev/guard.sh)
  cat > "$INSTALL_DIR/.telmivo-preview/compose.yml" <<'YAML'
services:
  miniapp:
    volumes:
      - ./.telmivo-preview/login.html:/usr/share/nginx/html/test-login.html:ro
YAML
  # Mount target exists before container creation; payload is refreshed after build.
  printf '<!doctype html><meta charset="utf-8"><p>Preparing preview login...</p>\n' > "$INSTALL_DIR/.telmivo-preview/login.html"
  chmod 644 "$INSTALL_DIR/.telmivo-preview/login.html"
  # Clear inherited shell overrides of Compose configuration and .env values.
  unset COMPOSE_FILE COMPOSE_PROJECT_NAME COMPOSE_PROFILES COMPOSE_ENV_FILES
  while IFS='=' read -r key _; do
    [[ "$key" =~ ^[A-Z][A-Z0-9_]*$ ]] && unset "$key"
  done < "$INSTALL_DIR/.env"
  say '۴/۶ — ساخت imageها و اجرای مهاجرت‌ها در PostgreSQL همین پروژه'
  compose config --quiet
  compose up --build -d
  say '۵/۶ — بررسی سلامت؛ ممکن است اولین ساخت چند دقیقه طول بکشد'
  ready=false
  for ((n=0;n<90;n++)); do
    if curl --connect-timeout 2 --max-time 5 -fsS http://127.0.0.1:8080/health/ready >/dev/null && curl --connect-timeout 2 --max-time 5 -fsS http://127.0.0.1:5173/ >/dev/null; then ready=true; break; fi
    sleep 2
  done
  if [[ "$ready" != true ]]; then compose ps; die 'healthcheck موفق نشد؛ logs را بررسی کن. هیچ volume پاک نشد.'; fi
  python3 - "$INSTALL_DIR/.telmivo-preview/login.html" <<'PY'
import hashlib,hmac,json,time,urllib.parse,sys
from pathlib import Path
fields={'auth_date':str(int(time.time())),'query_id':'preview-'+str(time.time_ns()),'user':json.dumps({'id':94000001,'first_name':'آزمایش TELMIVO'},ensure_ascii=False,separators=(',',':'))}
check='\n'.join(k+'='+fields[k] for k in sorted(fields))
key=hmac.new(b'WebAppData',b'synthetic-signing-only',hashlib.sha256).digest()
fields['hash']=hmac.new(key,check.encode(),hashlib.sha256).hexdigest()
payload=json.dumps({'init_data':urllib.parse.urlencode(fields)})
html='''<!doctype html><html lang="fa" dir="rtl"><meta charset="utf-8"><title>TELMIVO آزمایشی</title><body style="font-family:sans-serif;padding:2rem;background:#111827;color:white"><h1>ورود به TELMIVO آزمایشی</h1><p>هویت ساختگی؛ بدون ارتباط با تلگرام واقعی. این صفحه را بلافاصله پس از نصب باز کن.</p><button id="login">ورود آزمایشی</button><p id="status"></p><script>
const payload=PAYLOAD;
document.getElementById('login').onclick=async()=>{try{const r=await fetch('/api/v1/auth/telegram',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)});if(!r.ok)throw Error('ورود ناموفق؛ برای امضای تازه نصب‌کننده را دوباره اجرا کن.');location.href='/';}catch(e){document.getElementById('status').textContent=e.message;}};
</script></body></html>'''.replace('PAYLOAD',payload)
Path(sys.argv[1]).write_text(html)
PY
  chmod 644 "$INSTALL_DIR/.telmivo-preview/login.html"
  say '۶/۶ — نصب و healthcheck کامل شد؛ این نتیجه، PASS همه تست‌ها نیست'
  compose ps
  printf '\nمسیر نصب: %s\nSource SHA: %s\n' "$INSTALL_DIR" "$SOURCE_SHA"
  printf 'صفحه تست: http://127.0.0.1:5173/test-login.html\n'
  printf 'در Termius یک Local Tunnel بساز: local 5173 → destination 127.0.0.1:5173 روی همین سرور.\n'
  printf 'برای ورود با امضای تازه همین فرمان نصب را دوباره اجرا کن؛ داده‌ها حفظ می‌شوند.\n'
  printf 'Logs: sudo docker compose -p %s --project-directory "%s" -f "%s/docker-compose.yml" -f "%s/.telmivo-preview/compose.yml" logs --tail=80\n' "$PROJECT" "$INSTALL_DIR" "$INSTALL_DIR" "$INSTALL_DIR"
}
if [[ ${BASH_SOURCE[0]} == "$0" ]]; then main "$@"; fi
