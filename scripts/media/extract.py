"""Pinned extraction; no cookies, plugins, shell, generic downloader or network fallback."""
import http.client
import ipaddress
import json
import socket
import ssl
import sys
import urllib.parse
import io
from yt_dlp import YoutubeDL
from yt_dlp.version import __version__
from yt_dlp.networking.common import Response
from yt_dlp.networking.exceptions import TransportError, HTTPError

DOMAINS = {"youtube": ("youtube.com", "youtu.be", "googlevideo.com", "ytimg.com", "youtubei.googleapis.com"), "instagram": ("instagram.com", "cdninstagram.com", "fbcdn.net"), "facebook": ("facebook.com", "fbcdn.net", "fbsbx.com")}

def allowed(provider, raw):
    u = urllib.parse.urlsplit(raw)
    if u.scheme != "https" or u.username or u.password or u.port not in (None, 443) or u.fragment:
        raise ValueError("ACCESS_RESTRICTED")
    h = (u.hostname or "").lower()
    if not any(h == d or h.endswith("." + d) for d in DOMAINS.get(provider, ())):
        raise ValueError("ACCESS_RESTRICTED")
    addresses = sorted({v[4][0] for v in socket.getaddrinfo(h, 443, type=socket.SOCK_STREAM)})
    if not addresses or any(not ipaddress.ip_address(a).is_global for a in addresses):
        raise ValueError("ACCESS_RESTRICTED")
    return u, addresses[0]

class PinnedTLS(http.client.HTTPSConnection):
    def __init__(self, host, address):
        super().__init__(host, timeout=15, context=ssl.create_default_context())
        self.address = address
    def connect(self):
        self.sock = socket.create_connection((self.address, 443), timeout=15)
        self.sock = self._context.wrap_socket(self.sock, server_hostname=self.host)

class Silent:
    def debug(self, *args, **kwargs): pass
    def warning(self, *args, **kwargs): pass
    def error(self, *args, **kwargs): pass

class GuardedDL(YoutubeDL):
    def __init__(self, provider):
        self.provider = provider
        super().__init__({"quiet": True, "no_warnings": True, "logger": Silent(), "noplaylist": True, "skip_download": True, "cachedir": False, "socket_timeout": 15, "retries": 0, "extractor_retries": 0, "geo_bypass": False, "js_runtimes": {}, "remote_components": set(), "ignore_no_formats_error": False}, auto_init=False)
        self.add_default_info_extractors()
    def urlopen(self, req):
        raw = req if isinstance(req, str) else req.url
        data = getattr(req, "data", None)
        method = getattr(req, "method", "GET")
        headers = dict(getattr(req, "headers", {}))
        for name in list(headers):
            if name.lower() in ("cookie", "authorization", "proxy-authorization", "host"):
                headers.pop(name)
        try:
            for _ in range(4):
                u, address = allowed(self.provider, raw)
                conn = PinnedTLS(u.hostname, address)
                try:
                    conn.request(method, urllib.parse.urlunsplit(("", "", u.path or "/", u.query, "")), body=data, headers=headers)
                    reply = conn.getresponse()
                    if reply.status in (301, 302, 303, 307, 308):
                        raw = urllib.parse.urljoin(raw, reply.getheader("Location", ""))
                        if reply.status == 303: method, data = "GET", None
                        continue
                    body = reply.read((2 << 20) + 1)
                    if len(body) > 2 << 20: raise ValueError("ACCESS_RESTRICTED")
                    response = Response(io.BytesIO(body), raw, dict(reply.getheaders()), reply.status)
                    if reply.status >= 400: raise HTTPError(response)
                    return response
                finally:
                    conn.close()
            raise ValueError("ACCESS_RESTRICTED")
        except Exception as exc:
            if isinstance(exc, HTTPError): raise
            raise TransportError("restricted provider transport") from None

if __name__ == "__main__":
    if __version__ != "2026.08.19" or len(sys.argv) != 3 or sys.argv[1] not in DOMAINS:
        print(json.dumps({"error": "ANALYSIS_FAILED"})); sys.exit(0)
    provider, raw = sys.argv[1:]
    try:
        # Explicit extractor selection prevents generic/playlist/other-provider fallback.
        key = {"youtube": "Youtube", "instagram": "Instagram", "facebook": "Facebook"}[provider]
        with GuardedDL(provider) as ydl:
            info = ydl.extract_info(raw, download=False, ie_key=key)
            if info.get("_type") in ("playlist", "url", "url_transparent") or info.get("is_live") or info.get("age_limit", 0) >= 18 or info.get("availability") in ("private", "premium_only", "subscriber_only", "needs_auth"):
                raise ValueError("ACCESS_RESTRICTED")
            result = {"title": str(info.get("title", "media"))[:200], "duration": info.get("duration") or 0, "formats": []}
            for f in info.get("formats", [])[:200]:
                result["formats"].append({k: f.get(k) for k in ("format_id", "ext", "protocol", "url", "vcodec", "acodec", "filesize", "filesize_approx", "height", "has_drm")})
            print(json.dumps(result))
    except Exception as exc:
        # Error text is used privately only for category mapping, never emitted or logged.
        msg = str(exc).lower()
        code = "ACCESS_RESTRICTED" if any(v in msg for v in ("private", "login", "sign in", "captcha", "age", "not available in your country", "restricted", "drm", "premium")) else "PROVIDER_RATE_LIMITED" if "429" in msg else "MEDIA_NOT_FOUND" if "404" in msg else "PROVIDER_TEMPORARY_FAILURE"
        print(json.dumps({"error": code}))
