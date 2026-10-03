package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type Failure string

func (f Failure) Error() string { return string(f) }

const Invalid Failure = "INVALID_URL"
const Unsupported Failure = "UNSUPPORTED_PROVIDER"
const Restricted Failure = "ACCESS_RESTRICTED"
const TooLarge Failure = "MEDIA_TOO_LARGE"
const Unavailable Failure = "FORMAT_UNAVAILABLE"

type Source struct{ Provider, URL, Reference string }
type Format struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Kind       string `json:"kind"`
	Container  string `json:"container"`
	Codec      string `json:"codec"`
	Bytes      int64  `json:"bytes"`
	Processing bool   `json:"requires_processing"`
}
type Analysis struct {
	Title    string   `json:"title"`
	Duration float64  `json:"duration"`
	Formats  []Format `json:"formats"`
}
type Adapter interface {
	Analyze(context.Context, Source) (Analysis, error)
	Fetch(context.Context, Source, Format, io.Writer, int64) error
}

var youtubeID = regexp.MustCompile(`^[a-zA-Z0-9_-]{11}$`)
var postID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)

func Detect(raw string) (Source, error) {
	s := Source{}
	if len(raw) > 2048 || strings.ContainsAny(raw, "\x00\r\n\\") {
		return s, Invalid
	}
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" || u.RawPath != "" {
		return s, Invalid
	}
	h := strings.ToLower(u.Hostname())
	p := strings.Split(strings.Trim(u.Path, "/"), "/")
	switch h {
	case "youtube.com", "www.youtube.com", "m.youtube.com", "youtu.be":
		id := ""
		if h == "youtu.be" && len(p) == 1 {
			id = p[0]
		} else if u.Path == "/watch" {
			id = u.Query().Get("v")
		} else if len(p) == 2 && (p[0] == "shorts" || p[0] == "live") {
			id = p[1]
		}
		if !youtubeID.MatchString(id) {
			return s, Invalid
		}
		s = Source{"youtube", "https://www.youtube.com/watch?v=" + id, id}
	case "instagram.com", "www.instagram.com":
		if len(p) != 2 || (p[0] != "p" && p[0] != "reel" && p[0] != "tv") || !postID.MatchString(p[1]) {
			return s, Invalid
		}
		s = Source{"instagram", "https://www.instagram.com/" + p[0] + "/" + p[1] + "/", p[1]}
	case "facebook.com", "www.facebook.com", "m.facebook.com":
		id := ""
		if u.Path == "/watch/" || u.Path == "/watch" {
			id = u.Query().Get("v")
		} else if len(p) == 2 && p[0] == "reel" {
			id = p[1]
		} else if len(p) == 3 && p[1] == "videos" {
			id = p[2]
		}
		if !regexp.MustCompile(`^[0-9]{1,30}$`).MatchString(id) {
			return s, Invalid
		}
		s = Source{"facebook", "https://www.facebook.com/watch/?v=" + id, id}
	default:
		return s, Unsupported
	}
	return s, nil
}
func AllowedHost(provider, host string) bool {
	var domains []string
	switch provider {
	case "youtube":
		domains = []string{"youtube.com", "youtu.be", "googlevideo.com", "ytimg.com", "youtubei.googleapis.com"}
	case "instagram":
		domains = []string{"instagram.com", "cdninstagram.com", "fbcdn.net"}
	case "facebook":
		domains = []string{"facebook.com", "fbcdn.net", "fbsbx.com"}
	default:
		return false
	}
	for _, d := range domains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}
func PublicIP(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsGlobalUnicast() || a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() {
		return false
	}
	for _, raw := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "2001::/32", "2002::/16"} {
		if netip.MustParsePrefix(raw).Contains(a) {
			return false
		}
	}
	return true
}
func NetworkURL(provider, raw string) error {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" || !AllowedHost(provider, strings.ToLower(u.Hostname())) {
		return Restricted
	}
	if _, e := netip.ParseAddr(u.Hostname()); e == nil {
		return Restricted
	}
	return nil
}
func Client(provider string, timeout time.Duration, redirects int) *http.Client {
	tr := &http.Transport{Proxy: nil, DisableKeepAlives: true, ResponseHeaderTimeout: timeout, TLSHandshakeTimeout: timeout, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil || port != "443" || !AllowedHost(provider, strings.ToLower(host)) {
			return nil, Restricted
		}
		ips, e := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if e != nil || len(ips) == 0 {
			return nil, Restricted
		}
		for _, a := range ips {
			if !PublicIP(a) {
				return nil, Restricted
			}
		}
		d := net.Dialer{Timeout: timeout}
		return d.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}}
	return &http.Client{Transport: tr, Timeout: timeout, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > redirects {
			return Restricted
		}
		return NetworkURL(provider, r.URL.String())
	}}
}
func CopyBounded(ctx context.Context, w io.Writer, r io.Reader, max int64) (int64, error) {
	if max <= 0 {
		return 0, TooLarge
	}
	buf := make([]byte, 32<<10)
	var n int64
	for {
		if e := ctx.Err(); e != nil {
			return n, e
		}
		k, e := r.Read(buf)
		if k > 0 {
			if int64(k) > max-n {
				return n, TooLarge
			}
			written, we := w.Write(buf[:k])
			n += int64(written)
			if we != nil {
				return n, we
			}
			if written != k {
				return n, io.ErrShortWrite
			}
		}
		if e == io.EOF {
			return n, nil
		}
		if e != nil {
			return n, e
		}
	}
}

const ExtractorVersion = "2026.08.19"

type Extractor struct {
	Python, Script string
	Timeout        time.Duration
	MaxFormats     int
}
type rawFormat struct {
	ID       string `json:"format_id"`
	Ext      string `json:"ext"`
	Protocol string `json:"protocol"`
	URL      string `json:"url"`
	VCodec   string `json:"vcodec"`
	ACodec   string `json:"acodec"`
	Bytes    int64  `json:"filesize"`
	Height   int    `json:"height"`
	DRM      bool   `json:"has_drm"`
}
type rawInfo struct {
	Title    string      `json:"title"`
	Duration float64     `json:"duration"`
	Formats  []rawFormat `json:"formats"`
	Error    string      `json:"error"`
}
type boundedBuffer struct {
	b   []byte
	max int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.max-len(b.b) {
		return 0, TooLarge
	}
	b.b = append(b.b, p...)
	return len(p), nil
}
func (x Extractor) info(ctx context.Context, s Source) (rawInfo, error) {
	var info rawInfo
	clean, e := Detect(s.URL)
	if e != nil || clean != s {
		return info, Invalid
	}
	timeout := x.Timeout
	if timeout <= 0 || timeout > time.Minute {
		return info, Invalid
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, x.Python, "-I", x.Script, s.Provider, s.URL)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C.UTF-8"}
	out := &boundedBuffer{max: 2 << 20}
	cmd.Stdout = out
	cmd.Stderr = io.Discard
	if cmd.Run() != nil {
		return info, Failure("ANALYSIS_FAILED")
	}
	if json.Unmarshal(out.b, &info) != nil {
		return info, Failure("ANALYSIS_FAILED")
	}
	if info.Error != "" {
		switch info.Error {
		case "ACCESS_RESTRICTED", "MEDIA_NOT_FOUND", "PROVIDER_RATE_LIMITED", "PROVIDER_TEMPORARY_FAILURE":
			return info, Failure(info.Error)
		default:
			return info, Failure("ANALYSIS_FAILED")
		}
	}
	return info, nil
}
func descriptor(f rawFormat) (Format, bool) {
	if f.DRM || f.ID == "" || len(f.ID) > 100 || f.Protocol != "https" || (f.Ext != "mp4" && f.Ext != "m4a" && f.Ext != "mp3") || f.ACodec == "none" {
		return Format{}, false
	}
	kind := "video"
	if f.VCodec == "none" {
		kind = "audio"
	}
	sum := sha256.Sum256([]byte(f.ID + "\x00" + f.Ext + "\x00" + f.VCodec + "\x00" + f.ACodec))
	label := kind + " " + f.Ext
	if f.Height > 0 {
		label += " " + strings.TrimSpace(jsonNumber(f.Height)) + "p"
	}
	return Format{ID: hex.EncodeToString(sum[:12]), Label: label, Kind: kind, Container: f.Ext, Codec: f.VCodec + "/" + f.ACodec, Bytes: f.Bytes}, true
}
func jsonNumber(n int) string { b, _ := json.Marshal(n); return string(b) }
func SafeTitle(s string) string {
	var out strings.Builder
	for _, r := range s {
		if r > 31 && r != 127 && r != '/' && r != '\\' {
			out.WriteRune(r)
		}
		if out.Len() >= 200 {
			break
		}
	}
	if out.Len() == 0 {
		return "media"
	}
	return out.String()
}
func (x Extractor) Analyze(ctx context.Context, s Source) (Analysis, error) {
	info, e := x.info(ctx, s)
	a := Analysis{Title: SafeTitle(info.Title), Duration: info.Duration, Formats: []Format{}}
	if e != nil {
		return a, e
	}
	max := x.MaxFormats
	if max < 1 || max > 12 {
		return a, Invalid
	}
	seen := map[string]bool{}
	for _, f := range info.Formats {
		if len(a.Formats) >= max {
			break
		}
		v, ok := descriptor(f)
		if ok && !seen[v.ID] && NetworkURL(s.Provider, f.URL) == nil {
			seen[v.ID] = true
			a.Formats = append(a.Formats, v)
		}
	}
	if len(a.Formats) == 0 {
		return a, Unavailable
	}
	return a, nil
}
func (x Extractor) Fetch(ctx context.Context, s Source, selected Format, w io.Writer, max int64) error {
	info, e := x.info(ctx, s)
	if e != nil {
		return e
	}
	for _, f := range info.Formats {
		v, ok := descriptor(f)
		if !ok || v.ID != selected.ID {
			continue
		}
		if NetworkURL(s.Provider, f.URL) != nil {
			return Restricted
		}
		client := Client(s.Provider, time.Minute, 3)
		req, e := http.NewRequestWithContext(ctx, "GET", f.URL, nil)
		if e != nil {
			return Restricted
		}
		resp, e := client.Do(req)
		if e != nil {
			return Failure("DOWNLOAD_FAILED")
		}
		defer resp.Body.Close()
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return Restricted
		}
		if resp.StatusCode != 200 {
			return Failure("DOWNLOAD_FAILED")
		}
		_, e = CopyBounded(ctx, w, resp.Body, max)
		return e
	}
	return Unavailable
}
