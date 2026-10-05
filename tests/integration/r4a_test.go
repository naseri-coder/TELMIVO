//go:build integration

package integration

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	"github.com/naseri-coder/TELMIVO/internal/platform/config"
	"github.com/naseri-coder/TELMIVO/internal/platform/httpapi"
	files "github.com/naseri-coder/TELMIVO/internal/plugins/filemanager"
)

func TestR4ACanonicalRuntimeLifecycle(t *testing.T) {
	f := r3d4Fixture(t)
	scope := f.scope(files.ID)
	_, e := owner.Pool.Exec(ctx, "UPDATE customer_bots SET status='DRAFT' WHERE id=$1", f.bot)
	must(t, e)

	status, e := f.content.RuntimeStatus(ctx, f.u.UserID, scope)
	must(t, e)
	if status.Status != "DRAFT" || status.Connected || status.IngressPath != "/api/v1/customer-bot-updates/"+f.u.WorkspaceID+"/"+f.bot {
		t.Fatal("unexpected initial runtime", status)
	}
	if e = f.content.AdmitWebhook(ctx, scope); !errors.Is(e, contentplugins.ErrRuntimeNotRunning) {
		t.Fatal("draft bot admitted", e)
	}

	must(t, f.content.Connect(ctx, f.u.UserID, scope, "https://mini.test", "r4a-connect"))
	status, e = f.content.RuntimeStatus(ctx, f.u.UserID, scope)
	must(t, e)
	if status.Status != "RUNNING" || !status.Connected || status.Username != "r3d4_connected_test" {
		t.Fatal("connection not projected", status)
	}
	must(t, f.content.AdmitWebhook(ctx, scope))

	api := (&httpapi.API{DB: db, Identity: r3aService(), Content: f.content, Config: config.Config{Environment: "test", Synthetic: true, MiniAppURL: "https://mini.test"}}).Handler()
	call := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", status.IngressPath, strings.NewReader(`{}`))
		r.Header.Set("X-Telegram-Bot-Api-Secret-Token", f.content.WebhookSecret(f.u.WorkspaceID, f.bot))
		w := httptest.NewRecorder()
		api.ServeHTTP(w, r)
		return w
	}
	if w := call(); w.Code != 400 {
		t.Fatal("running ingress did not reach update validation", w.Code, w.Body.String())
	}

	must(t, f.content.Disconnect(ctx, f.u.UserID, scope, "r4a-disconnect"))
	status, e = f.content.RuntimeStatus(ctx, f.u.UserID, scope)
	must(t, e)
	if status.Status != "DISABLED" || status.Connected || f.transport.disconnects.Load() != 1 {
		t.Fatal("pause not durable", status, f.transport.disconnects.Load())
	}
	if w := call(); w.Code != 409 || !strings.Contains(w.Body.String(), "BOT_RUNTIME_NOT_RUNNING") {
		t.Fatal("paused ingress admitted", w.Code, w.Body.String())
	}
	must(t, f.content.Disconnect(ctx, f.u.UserID, scope, "r4a-disconnect-replay"))
	if f.transport.disconnects.Load() != 1 {
		t.Fatal("idempotent pause repeated external effect")
	}

	must(t, f.content.Connect(ctx, f.u.UserID, scope, "https://mini.test", "r4a-reconnect"))
	status, e = f.content.RuntimeStatus(ctx, f.u.UserID, scope)
	must(t, e)
	if status.Status != "RUNNING" || !status.Connected {
		t.Fatal("disabled bot could not be explicitly reconnected", status)
	}
}
