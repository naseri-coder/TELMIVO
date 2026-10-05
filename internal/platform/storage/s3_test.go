package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestS3CompatiblePrivateProvider(t *testing.T) {
	var mu sync.Mutex
	objects := map[string][]byte{}
	meta := map[string]http.Header{}
	var sawAuth bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") && r.URL.Query().Get("X-Amz-Algorithm") == "" {
			t.Error("unsigned request")
		}
		if strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			sawAuth = true
		}
		key := r.URL.Path
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			if int64(len(b)) != r.ContentLength {
				t.Error("content length mismatch")
			}
			objects[key] = b
			h := make(http.Header)
			h.Set("Content-Type", r.Header.Get("Content-Type"))
			h.Set("X-Amz-Meta-Expires-At", r.Header.Get("X-Amz-Meta-Expires-At"))
			sum := sha256.Sum256(b)
			h.Set("ETag", `"`+hex.EncodeToString(sum[:])+`"`)
			meta[key] = h
			w.WriteHeader(http.StatusOK)
		case http.MethodHead:
			b, ok := objects[key]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			for k, values := range meta[key] {
				for _, v := range values {
					w.Header().Add(k, v)
				}
			}
			w.Header().Set("Content-Length", stringInt64(int64(len(b))))
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			b, ok := objects[key]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write(b)
		case http.MethodDelete:
			delete(objects, key)
			delete(meta, key)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	s, e := NewS3(server.URL, "private-bucket", "eu-test-1", "access", "secret", server.Client())
	if e != nil {
		t.Fatal(e)
	}
	fixed := time.Date(2027, 1, 2, 3, 4, 5, 0, time.UTC)
	s.Now = func() time.Time { return fixed }
	scoped, e := NewS3Scoped(s)
	if e != nil {
		t.Fatal(e)
	}
	key, e := ObjectKey("11111111-1111-1111-1111-111111111111", "r4c")
	if e != nil {
		t.Fatal(e)
	}
	body := []byte("private uploader object")
	expires := fixed.Add(time.Hour)
	if e = scoped.Put(context.Background(), "11111111-1111-1111-1111-111111111111", key, bytes.NewReader(body), Metadata{Size: int64(len(body)), ContentType: "text/plain", ExpiresAt: expires}); e != nil {
		t.Fatal(e)
	}
	m, e := scoped.Metadata(context.Background(), "11111111-1111-1111-1111-111111111111", key)
	if e != nil || m.Size != int64(len(body)) || m.ContentType != "text/plain" {
		t.Fatal(m, e)
	}
	r, e := scoped.Open(context.Background(), "11111111-1111-1111-1111-111111111111", key)
	if e != nil {
		t.Fatal(e)
	}
	got, _ := io.ReadAll(r)
	_ = r.Close()
	if !bytes.Equal(got, body) || !sawAuth {
		t.Fatal("object round trip failed")
	}
	u, e := scoped.SignedURL(context.Background(), "11111111-1111-1111-1111-111111111111", key, 5*time.Minute)
	if e != nil || !strings.Contains(u, "X-Amz-Signature=") || !strings.Contains(u, "X-Amz-Credential=") {
		t.Fatal(u, e)
	}
	if e = scoped.Delete(context.Background(), "11111111-1111-1111-1111-111111111111", key); e != nil {
		t.Fatal(e)
	}
}

func TestS3RejectsUnsafeConfiguration(t *testing.T) {
	for _, endpoint := range []string{"http://example.com", "https://user:pass@example.com", "https://example.com?x=1"} {
		if _, e := NewS3(endpoint, "private-bucket", "eu-test-1", "access", "secret", nil); e == nil {
			t.Fatal("unsafe endpoint accepted", endpoint)
		}
	}
	if _, e := NewS3("https://example.com", "BAD_BUCKET", "eu-test-1", "access", "secret", nil); e == nil {
		t.Fatal("unsafe bucket accepted")
	}
}

func stringInt64(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [32]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
