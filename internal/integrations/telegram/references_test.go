package telegram

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestReferenceTransport(t *testing.T) {
	c := New("synthetic-reference-only", true)
	state := "member"
	reject := 0
	methods := []string{}
	c.http = &http.Client{Transport: groupRoundTrip(func(r *http.Request) (*http.Response, error) {
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		methods = append(methods, method)
		body := `{"ok":true,"result":{"message_id":101}}`
		if method == "getMe" {
			body = `{"ok":true,"result":{"id":999,"is_bot":true,"username":"synthetic"}}`
		} else if method == "deleteWebhook" {
			body = `{"ok":true,"result":true}`
		} else if method == "getChatMember" {
			body = `{"ok":true,"result":{"status":"` + state + `","is_member":true}}`
			if len(methods)%3 == 2 {
				body = `{"ok":true,"result":{"status":"administrator"}}`
			}
		}
		if reject == 403 {
			body = `{"ok":false,"error_code":403}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}
	st, code := c.Check(context.Background(), -1000000000001, 10)
	if st != "MEMBER" || code != "" {
		t.Fatal(st, code)
	}
	reject = 403
	st, code = c.Check(context.Background(), -1000000000001, 10)
	if st != "CHECK_FAILED" || code != "BOT_NOT_AUTHORIZED" {
		t.Fatal(st, code)
	}
	reject = 0
	for _, kind := range []string{"document", "video", "audio", "photo"} {
		id, code := c.SendFile(context.Background(), 10, kind, "opaque-reference", "title")
		if id != 101 || code != "DELIVERY_ACKNOWLEDGED" {
			t.Fatal(kind, code)
		}
	}
	if e := c.Disconnect(context.Background()); e != nil {
		t.Fatal(e)
	}
	if methods[len(methods)-1] != "deleteWebhook" {
		t.Fatal(methods)
	}
	c.Enabled = false
	if _, code = c.SendFile(context.Background(), 10, "document", "ref", "title"); code != "DELIVERY_FAILED" {
		t.Fatal(code)
	}
}
