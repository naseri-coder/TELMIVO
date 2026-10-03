package media

import (
	"bytes"
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProviderBoundary(t *testing.T) {
	for _, c := range []struct{ url, provider string }{{"https://youtu.be/dQw4w9WgXcQ", "youtube"}, {"https://www.youtube.com/watch?v=dQw4w9WgXcQ&token=secret", "youtube"}, {"https://www.instagram.com/reel/abc123/", "instagram"}, {"https://www.facebook.com/watch/?v=1234", "facebook"}} {
		s, e := Detect(c.url)
		if e != nil || s.Provider != c.provider {
			t.Fatal(c.url, s, e)
		}
		if s.Provider != "youtube" && bytes.Contains([]byte(s.URL), []byte("token")) {
			t.Fatal("query survived")
		}
	}
	for _, s := range []string{"http://youtube.com/watch?v=dQw4w9WgXcQ", "https://youtube.com.attacker.test/watch?v=dQw4w9WgXcQ", "https://localhost/a", "https://127.0.0.1/a", "https://169.254.169.254/latest/meta-data", "https://youtube.com@127.0.0.1/", "https://youtube.com:8443/watch?v=dQw4w9WgXcQ", "https://instagram.com/accounts/login/", "https://youtube.com/playlist?list=foo", "https://facebook.com/profile.php?id=3", "file:///etc/passwd", "https://youtube.com/%2e%2e/a", "https://[::1]/a", "https://10.0.0.1/a", "https://youtu.be/x#token"} {
		if _, e := Detect(s); e == nil {
			t.Fatal("accepted", s)
		}
	}
}
func TestSSRFGuard(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.1.1.1", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.100.100.200", "::1", "fc00::1", "fe80::1", "::ffff:127.0.0.1", "198.18.0.1", "2001:db8::1"} {
		if PublicIP(netip.MustParseAddr(s)) {
			t.Fatal(s)
		}
	}
	if !PublicIP(netip.MustParseAddr("8.8.8.8")) {
		t.Fatal("public blocked")
	}
	for _, s := range []string{"https://evil.test/file", "https://googlevideo.com.evil.test/a", "https://googlevideo.com@169.254.169.254/a", "http://googlevideo.com/a"} {
		if NetworkURL("youtube", s) == nil {
			t.Fatal(s)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	tr := Client("youtube", time.Second, 2).Transport
	_ = tr
	if _, e := Client("youtube", time.Second, 2).Get("https://127.0.0.1/a"); e == nil {
		t.Fatal("dialed private literal")
	}
	if ctx.Err() != nil {
		t.Fatal("unexpected wait")
	}
}
func TestActualByteCap(t *testing.T) {
	for _, n := range []int{4, 5, 6} {
		var out bytes.Buffer
		size, e := CopyBounded(context.Background(), &out, bytes.NewReader(bytes.Repeat([]byte{'x'}, n)), 5)
		if n <= 5 && (e != nil || size != int64(n)) {
			t.Fatal(n, e)
		}
		if n > 5 && !errors.Is(e, TooLarge) {
			t.Fatal("overrun", e)
		}
		if out.Len() > 5 {
			t.Fatal("wrote beyond cap")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := CopyBounded(ctx, &bytes.Buffer{}, bytes.NewReader([]byte("x")), 5); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
func TestMetadataFormatBoundary(t *testing.T) {
	r := rawFormat{ID: "18;touch /tmp/pwn", Ext: "mp4", Protocol: "https", VCodec: "avc1", ACodec: "mp4a", Height: 360}
	f, ok := descriptor(r)
	if !ok || len(f.ID) != 24 || f.Processing {
		t.Fatal(f)
	}
	other, _ := descriptor(r)
	if other.ID != f.ID {
		t.Fatal("unstable")
	}
	r.DRM = true
	if _, ok := descriptor(r); ok {
		t.Fatal("DRM accepted")
	}
	if SafeTitle("../\x00evil/video") != "..evilvideo" {
		t.Fatal("unsafe title")
	}
}
func TestMediaValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-media.mp4")
	if e := os.WriteFile(path, []byte("not media"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Validate(context.Background(), path, 100, Format{Kind: "video", Container: "mp4"}); e == nil {
		t.Fatal("malformed accepted")
	}
	fixture, err := os.ReadFile("../../platform/testfixtures/media/compatible.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, fixture, 0600); err != nil {
		t.Fatal(err)
	}
	a, e := Validate(context.Background(), path, 1<<20, Format{Kind: "video", Container: "mp4"})
	if e != nil || a.Bytes != int64(len(fixture)) {
		t.Fatal(a, e)
	}
}
