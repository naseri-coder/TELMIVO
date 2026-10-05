package telegram

import (
	"context"
	"errors"
	"github.com/naseri-coder/TELMIVO/internal/integrations/media"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type mediaRoundTrip func(*http.Request) (*http.Response, error)

func (f mediaRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestMediaTransportOutcomes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "asset")
	if e := os.WriteFile(path, []byte("abc"), 0600); e != nil {
		t.Fatal(e)
	}
	cases := []struct {
		name, body, want string
		status           int
		retry, network   bool
	}{
		{"ack", `{"ok":true,"result":{"message_id":7}}`, "DELIVERY_ACKNOWLEDGED", 200, false, false},
		{"permanent", `{"ok":false,"error_code":400}`, "DELIVERY_FAILED", 400, false, false},
		{"temporary", `{"ok":false,"error_code":429}`, "DELIVERY_FAILED", 429, true, false},
		{"malformed", `broken`, "DELIVERY_OUTCOME_UNKNOWN", 200, false, false},
		{"ambiguous", "", "DELIVERY_OUTCOME_UNKNOWN", 0, false, true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			client := MediaClient{Token: "synthetic-token", MaxBytes: 100, Timeout: time.Second, transport: mediaRoundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "api.telegram.org" || !strings.HasSuffix(r.URL.Path, "/sendVideo") {
					t.Fatal("method/target")
				}
				parts, e := r.MultipartReader()
				if e != nil {
					return nil, e
				}
				chat, file := false, false
				for {
					p, e := parts.NextPart()
					if e == io.EOF {
						break
					}
					if e != nil {
						return nil, e
					}
					b, e := io.ReadAll(p)
					if e != nil {
						return nil, e
					}
					if p.FormName() == "chat_id" {
						chat = string(b) == "42"
					}
					if p.FormName() == "video" {
						file = string(b) == "abc"
					}
				}
				if !chat || !file {
					t.Fatal("wrong multipart target or asset")
				}
				if test.network {
					return nil, errors.New("synthetic network ambiguity")
				}
				return &http.Response{StatusCode: test.status, Body: io.NopCloser(strings.NewReader(test.body)), Header: http.Header{}, Request: r}, nil
			})}
			result, e := client.DeliverMedia(context.Background(), 42, media.Asset{Path: path, Name: "media.mp4", Kind: "video", Bytes: 3})
			if e != nil || result.Outcome != test.want || result.Retry != test.retry {
				t.Fatal(result, e)
			}
		})
	}
}
