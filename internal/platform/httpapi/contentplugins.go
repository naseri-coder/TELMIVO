package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	platform "github.com/naseri-coder/TELMIVO/internal/domains/pluginplatform"
	tg "github.com/naseri-coder/TELMIVO/internal/integrations/telegram"
	cred "github.com/naseri-coder/TELMIVO/internal/platform/crypto"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/platform/testfixtures"
	gate "github.com/naseri-coder/TELMIVO/internal/plugins/accessgate"
	files "github.com/naseri-coder/TELMIVO/internal/plugins/filemanager"
	"net/http"
)

type contentProvider struct {
	keys    cred.CredentialStore
	enabled bool
}

func (p contentProvider) ForBot(ctx context.Context, tx pgx.Tx, scope sdk.Scope) (contentplugins.Transport, error) {
	if p.keys == nil {
		return nil, sdk.ErrPermission
	}
	var secret cred.EncryptedSecret
	if e := tx.QueryRow(ctx, "SELECT key_id,encrypted_token FROM customer_bots WHERE workspace_id=$1 AND id::text=$2", scope.AccountID, scope.BotID).Scan(&secret.KeyID, &secret.Ciphertext); e != nil {
		return nil, e
	}
	b, e := p.keys.Open(ctx, scope.AccountID+":"+scope.BotID, secret)
	if e != nil {
		return nil, sdk.ErrPermission
	}
	return tg.New(string(b), p.enabled), nil
}
func (a *API) content() *contentplugins.Service {
	if a.Content != nil {
		return a.Content
	}
	f := a.factory()
	key, _ := base64.StdEncoding.DecodeString(a.Config.CredentialKey)
	var provider contentplugins.Provider = contentProvider{f.Credentials, !a.Config.Synthetic}
	if a.Config.ContentTestFixture && a.Config.Environment == "test" && a.Config.Synthetic {
		provider = testfixtures.ContentProvider{}
	}
	return &contentplugins.Service{Support: a.BusinessService(), DB: a.DB, Core: &platform.Service{DB: a.DB, Registry: f.Registry, Policy: f.Commercial, Hooks: f.Hooks}, Commercial: f.Commercial, Provider: provider, DigestKey: key}
}
func contentScope(r *http.Request, id string) sdk.Scope {
	return sdk.Scope{AccountID: workspace(r), BotID: r.PathValue("bot"), PluginID: id}
}
func (a *API) contentPolicies(w http.ResponseWriter, r *http.Request) {
	out, e := a.content().Policies(r.Context(), user(r), contentScope(r, gate.ID))
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
func (a *API) contentPolicySave(w http.ResponseWriter, r *http.Request) {
	var in contentplugins.PolicyChange
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	out, e := a.content().SavePolicy(r.Context(), user(r), contentScope(r, gate.ID), r.PathValue("policy"), correlation(r), in)
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
func (a *API) contentFiles(w http.ResponseWriter, r *http.Request) {
	out, e := a.content().Files(r.Context(), user(r), contentScope(r, files.ID), r.URL.Query().Get("search"))
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
func (a *API) contentFileSave(w http.ResponseWriter, r *http.Request) {
	var in contentplugins.FileChange
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	out, e := a.content().SaveFile(r.Context(), user(r), contentScope(r, files.ID), r.PathValue("reference"), correlation(r), in)
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
func (a *API) contentConnect(w http.ResponseWriter, r *http.Request) {
	var in struct{}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	if e := a.content().Connect(r.Context(), user(r), contentScope(r, files.ID), a.Config.MiniAppURL, correlation(r)); e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, map[string]bool{"connected": true})
}
func (a *API) contentWebhook(w http.ResponseWriter, r *http.Request) {
	s := a.content()
	account, bot := r.PathValue("account"), r.PathValue("bot")
	if len(s.DigestKey) < 32 || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Telegram-Bot-Api-Secret-Token")), []byte(s.WebhookSecret(account, bot))) != 1 {
		fail(w, 403, "forbidden")
		return
	}
	var u contentplugins.Update
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&u) != nil {
		fail(w, 400, "invalid")
		return
	}
	if e := s.HandleUpdate(r.Context(), sdk.Scope{AccountID: account, BotID: bot, PluginID: files.ID}, "customer-telegram-update", u); e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, map[string]bool{"accepted": true})
}

func (a *API) contentPolicyCheck(w http.ResponseWriter, r *http.Request) {
	var in struct {
		User int64 `json:"user"`
	}
	if decode(w, r, &in) != nil || in.User <= 0 {
		fail(w, 400, "invalid")
		return
	}
	s := a.content()
	scope := contentScope(r, gate.ID)
	runtime := &platform.Runtime{Service: s.Core, Handler: s}
	d, e := (gate.Plugin{Runtime: runtime.Bind(user(r), scope)}).Authorize(r.Context(), r.PathValue("policy"), in.User)
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, d)
}
