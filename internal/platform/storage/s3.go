package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var bucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

type S3 struct {
	Endpoint  *url.URL
	Bucket    string
	Region    string
	AccessKey string
	SecretKey string
	Client    *http.Client
	Now       func() time.Time
}

func NewS3(endpoint, bucket, region, accessKey, secretKey string, client *http.Client) (*S3, error) {
	u, e := url.Parse(endpoint)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		!bucketPattern.MatchString(bucket) || strings.Contains(bucket, "..") || region == "" || len(region) > 64 ||
		accessKey == "" || secretKey == "" {
		return nil, ErrDenied
	}
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	return &S3{Endpoint: u, Bucket: bucket, Region: region, AccessKey: accessKey, SecretKey: secretKey, Client: client, Now: time.Now}, nil
}

func (s *S3) PrivateBucket() bool { return true }

func (s *S3) objectURL(key string) (*url.URL, error) {
	if !validKey(key) {
		return nil, ErrDenied
	}
	u := *s.Endpoint
	base := strings.TrimSuffix(u.Path, "/")
	parts := strings.Split(key, "/")
	u.Path = base + "/" + s.Bucket + "/" + strings.Join(parts, "/")
	return &u, nil
}

func hmacSHA(key []byte, value string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(value))
	return h.Sum(nil)
}

func awsEscape(v string) string {
	return strings.ReplaceAll(url.QueryEscape(v), "+", "%20")
}

func canonicalQuery(v url.Values) string {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	var b strings.Builder
	first := true
	for _, k := range keys {
		values := append([]string(nil), v[k]...)
		for i := 0; i < len(values); i++ {
			for j := i + 1; j < len(values); j++ {
				if values[j] < values[i] {
					values[i], values[j] = values[j], values[i]
				}
			}
		}
		for _, value := range values {
			if !first {
				b.WriteByte('&')
			}
			first = false
			b.WriteString(awsEscape(k))
			b.WriteByte('=')
			b.WriteString(awsEscape(value))
		}
	}
	return b.String()
}

func (s *S3) signingKey(day string) []byte {
	date := hmacSHA([]byte("AWS4"+s.SecretKey), day)
	region := hmacSHA(date, s.Region)
	service := hmacSHA(region, "s3")
	return hmacSHA(service, "aws4_request")
}

func (s *S3) sign(req *http.Request, payloadHash string) {
	now := s.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	day := now.Format("20060102")
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	canonicalHeaders := "host:" + req.URL.Host + "\n" +
		"x-amz-content-sha256:" + payloadHash + "\n" +
		"x-amz-date:" + amzDate + "\n"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonical := req.Method + "\n" + req.URL.EscapedPath() + "\n" + canonicalQuery(req.URL.Query()) + "\n" + canonicalHeaders + "\n" + signedHeaders + "\n" + payloadHash
	sum := sha256.Sum256([]byte(canonical))
	scope := day + "/" + s.Region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(sum[:])
	sig := hex.EncodeToString(hmacSHA(s.signingKey(day), stringToSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+s.AccessKey+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+sig)
}

func (s *S3) request(ctx context.Context, method, key string, body io.Reader, m Metadata) (*http.Response, error) {
	u, e := s.objectURL(key)
	if e != nil {
		return nil, e
	}
	req, e := http.NewRequestWithContext(ctx, method, u.String(), body)
	if e != nil {
		return nil, ErrUnavailable
	}
	if method == http.MethodPut {
		if m.Size < 0 || len(m.ContentType) > 256 {
			return nil, ErrDenied
		}
		req.ContentLength = m.Size
		req.Header.Set("Content-Type", m.ContentType)
		if !m.ExpiresAt.IsZero() {
			req.Header.Set("X-Amz-Meta-Expires-At", m.ExpiresAt.UTC().Format(time.RFC3339Nano))
		}
	}
	s.sign(req, "UNSIGNED-PAYLOAD")
	resp, e := s.Client.Do(req)
	if e != nil {
		return nil, ErrUnavailable
	}
	return resp, nil
}

func (s *S3) Put(ctx context.Context, key string, r io.Reader, m Metadata) error {
	if m.Size < 0 {
		return ErrDenied
	}
	resp, e := s.request(ctx, http.MethodPut, key, r, m)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ErrUnavailable
	}
	return nil
}

func (s *S3) Metadata(ctx context.Context, key string) (Metadata, error) {
	resp, e := s.request(ctx, http.MethodHead, key, nil, Metadata{})
	if e != nil {
		return Metadata{}, e
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return Metadata{}, ErrUnavailable
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Metadata{}, ErrUnavailable
	}
	size, e := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	if e != nil || size < 0 {
		return Metadata{}, ErrUnavailable
	}
	m := Metadata{Size: size, ContentType: resp.Header.Get("Content-Type"), ETag: strings.Trim(resp.Header.Get("ETag"), "\"")}
	if raw := resp.Header.Get("X-Amz-Meta-Expires-At"); raw != "" {
		m.ExpiresAt, e = time.Parse(time.RFC3339Nano, raw)
		if e != nil {
			return Metadata{}, ErrUnavailable
		}
		if !s.Now().Before(m.ExpiresAt) {
			return Metadata{}, ErrDenied
		}
	}
	return m, nil
}

func (s *S3) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if _, e := s.Metadata(ctx, key); e != nil {
		return nil, e
	}
	resp, e := s.request(ctx, http.MethodGet, key, nil, Metadata{})
	if e != nil {
		return nil, e
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, ErrUnavailable
	}
	return resp.Body, nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	resp, e := s.request(ctx, http.MethodDelete, key, nil, Metadata{})
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || (resp.StatusCode >= 200 && resp.StatusCode < 300) {
		return nil
	}
	return ErrUnavailable
}

func (s *S3) SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if e := ctx.Err(); e != nil {
		return "", e
	}
	if ttl <= 0 || ttl > time.Hour {
		return "", ErrDenied
	}
	u, e := s.objectURL(key)
	if e != nil {
		return "", e
	}
	now := s.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	day := now.Format("20060102")
	scope := day + "/" + s.Region + "/s3/aws4_request"
	q := u.Query()
	q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	q.Set("X-Amz-Credential", s.AccessKey+"/"+scope)
	q.Set("X-Amz-Date", amzDate)
	q.Set("X-Amz-Expires", strconv.FormatInt(int64(ttl/time.Second), 10))
	q.Set("X-Amz-SignedHeaders", "host")
	u.RawQuery = canonicalQuery(q)
	canonical := "GET\n" + u.EscapedPath() + "\n" + u.RawQuery + "\nhost:" + u.Host + "\n\nhost\nUNSIGNED-PAYLOAD"
	sum := sha256.Sum256([]byte(canonical))
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(sum[:])
	sig := hex.EncodeToString(hmacSHA(s.signingKey(day), stringToSign))
	u.RawQuery += "&X-Amz-Signature=" + sig
	return u.String(), nil
}

var _ S3Compatible = (*S3)(nil)

func (s *S3) String() string {
	return fmt.Sprintf("s3-compatible://%s/%s", s.Endpoint.Host, s.Bucket)
}

var _ = errors.Is
