package telegram

import (
	"context"
	"encoding/json"
	"github.com/naseri-coder/TELMIVO/internal/domains/groupbot"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type groupRoundTrip func(*http.Request) (*http.Response, error)

func (f groupRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestGroupTelegramBoundary(t *testing.T) {
	c := New("synthetic-boundary", true)
	methods := []string{}
	c.http = &http.Client{Transport: groupRoundTrip(func(r *http.Request) (*http.Response, error) {
		methods = append(methods, r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:])
		var p map[string]any
		if json.NewDecoder(r.Body).Decode(&p) != nil {
			t.Fatal("payload")
		}
		method := methods[len(methods)-1]
		result := `true`
		if method == "getMe" {
			result = `{"id":999}`
		}
		if method == "getChatMember" {
			result = `{"status":"administrator","can_delete_messages":true,"can_restrict_members":true}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":` + result + `}`)), Header: http.Header{}}, nil
	})}
	id, m, e := c.Bot(context.Background(), -123)
	if e != nil || id != 999 || !m.Admin() || !m.Delete || !m.Restrict {
		t.Fatal(id, m, e)
	}
	for _, a := range []groupbot.Action{groupbot.Delete, groupbot.Restrict, groupbot.Ban, groupbot.Unmute, groupbot.Unban} {
		if e = c.Execute(context.Background(), -123, 10, 11, a, 1000); e != nil {
			t.Fatal(e)
		}
	}
	if len(methods) != 7 {
		t.Fatal(methods)
	}
	c.http.Transport = groupRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(`{"ok":false,"error_code":400}`)), Header: http.Header{}}, nil
	})
	if _, e = c.Inspect(context.Background(), -123, 10); e == nil {
		t.Fatal("uncertainty allowed")
	}
}
func TestGroupWebhookMalformed(t *testing.T) {
	h := GroupBotWebhook("synthetic-webhook-fixture", &groupbot.Service{})
	for _, f := range []struct {
		body, secret string
		status       int
	}{{`{}`, "bad", 403}, {`{`, "synthetic-webhook-fixture", 400}, {`{}`, "synthetic-webhook-fixture", 400}, {`{"update_id":1} {}`, "synthetic-webhook-fixture", 400}} {
		r := httptest.NewRequest("POST", "/webhook", strings.NewReader(f.body))
		r.Header.Set("X-Telegram-Bot-Api-Secret-Token", f.secret)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != f.status {
			t.Fatal(w.Code)
		}
	}
}
