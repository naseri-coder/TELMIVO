package httpapi

import (
	"encoding/base64"
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	"github.com/naseri-coder/TELMIVO/internal/domains/intelligence"
	platform "github.com/naseri-coder/TELMIVO/internal/domains/pluginplatform"
	provider "github.com/naseri-coder/TELMIVO/internal/platform/ai"
	"github.com/naseri-coder/TELMIVO/internal/platform/testfixtures"
	assistant "github.com/naseri-coder/TELMIVO/internal/plugins/aiassistant"
	workflow "github.com/naseri-coder/TELMIVO/internal/plugins/automation"
	"net/http"
)

func (a *API) IntelligenceService() *intelligence.Service {
	if a.Intelligence != nil {
		return a.Intelligence
	}
	f := a.factory()
	support := a.support()
	core := &platform.Service{DB: a.DB, Registry: f.Registry, Policy: f.Commercial, Hooks: f.Hooks}
	key, _ := base64.StdEncoding.DecodeString(a.Config.CredentialKey)
	content := &contentplugins.Service{DB: a.DB, Core: core, Commercial: f.Commercial, Provider: support.Provider, DigestKey: key}
	synthetic := a.Config.Environment == "test" && a.Config.Synthetic && a.Config.ContentTestFixture
	var ai provider.Provider = provider.HTTPProvider{Credentials: f.Credentials}
	if synthetic {
		ai = testfixtures.AIProvider{}
	}
	return &intelligence.Service{DB: a.DB, Core: core, Commercial: f.Commercial, Support: support, Content: content, Provider: ai, Credentials: f.Credentials, Clock: f.Commercial.Clock, Synthetic: synthetic}
}
func (a *API) intelligenceWorkflows(w http.ResponseWriter, r *http.Request) {
	v, e := a.IntelligenceService().Workflows(r.Context(), user(r), contentScope(r, workflow.ID), r.URL.Query().Get("cursor"))
	supportReply(w, v, e)
}
func (a *API) intelligenceWorkflowSave(w http.ResponseWriter, r *http.Request) {
	var in intelligence.WorkflowChange
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	v, e := a.IntelligenceService().SaveWorkflow(r.Context(), user(r), contentScope(r, workflow.ID), r.PathValue("workflow"), in)
	supportReply(w, v, e)
}
func (a *API) intelligencePreview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Workflow workflow.Workflow   `json:"workflow"`
		Data     map[string][]string `json:"data"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	v, e := a.IntelligenceService().Preview(r.Context(), user(r), contentScope(r, workflow.ID), in.Workflow, in.Data)
	supportReply(w, v, e)
}
func (a *API) intelligenceManual(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Key     string `json:"idempotency_key"`
		Ticket  string `json:"ticket"`
		Contact string `json:"contact"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	v, e := a.IntelligenceService().Manual(r.Context(), user(r), contentScope(r, workflow.ID), r.PathValue("workflow"), in.Key, in.Ticket, in.Contact)
	supportReply(w, map[string]string{"execution_id": v}, e)
}
func (a *API) intelligenceRun(w http.ResponseWriter, r *http.Request) {
	var in struct{}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	e := (workflow.Plugin{Runtime: a.IntelligenceService().Bind(user(r), contentScope(r, workflow.ID))}).Run(r.Context(), r.PathValue("execution"))
	supportReply(w, map[string]bool{"completed": e == nil}, e)
}
func (a *API) intelligenceHistory(w http.ResponseWriter, r *http.Request) {
	v, e := a.IntelligenceService().History(r.Context(), user(r), contentScope(r, workflow.ID), r.URL.Query().Get("cursor"))
	supportReply(w, v, e)
}
func (a *API) intelligenceModels(w http.ResponseWriter, r *http.Request) {
	v, e := a.IntelligenceService().Models(r.Context(), user(r), contentScope(r, assistant.ID))
	supportReply(w, v, e)
}
func (a *API) intelligenceModelSave(w http.ResponseWriter, r *http.Request) {
	var in intelligence.ModelChange
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	v, e := a.IntelligenceService().SaveModel(r.Context(), user(r), workspace(r), in)
	supportReply(w, v, e)
}
func (a *API) intelligenceConfig(w http.ResponseWriter, r *http.Request) {
	v, e := a.IntelligenceService().Config(r.Context(), user(r), contentScope(r, assistant.ID))
	supportReply(w, v, e)
}
func (a *API) intelligenceConfigSave(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Config   assistant.Config `json:"configuration"`
		Expected int64            `json:"expected_revision"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	v, e := a.IntelligenceService().SaveConfig(r.Context(), user(r), contentScope(r, assistant.ID), in.Config, in.Expected)
	supportReply(w, v, e)
}
func (a *API) intelligenceKnowledge(w http.ResponseWriter, r *http.Request) {
	v, e := a.IntelligenceService().Knowledge(r.Context(), user(r), contentScope(r, assistant.ID))
	supportReply(w, v, e)
}
func (a *API) intelligenceKnowledgeSave(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Knowledge intelligence.Knowledge `json:"knowledge"`
		Key       string                 `json:"idempotency_key"`
		Expected  int64                  `json:"expected_revision"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	v, e := a.IntelligenceService().SaveKnowledge(r.Context(), user(r), contentScope(r, assistant.ID), in.Knowledge, in.Key, in.Expected)
	supportReply(w, v, e)
}
func (a *API) intelligenceChat(w http.ResponseWriter, r *http.Request) {
	var in intelligence.Chat
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	in.Purpose = "PLAYGROUND"
	if in.Ticket != "" {
		in.Purpose = "AGENT_ASSIST"
	}
	v, e := a.IntelligenceService().Chat(r.Context(), user(r), contentScope(r, assistant.ID), in)
	supportReply(w, v, e)
}
func (a *API) intelligenceReset(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Expected int64 `json:"expected_revision"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	e := a.IntelligenceService().ResetThread(r.Context(), user(r), contentScope(r, assistant.ID), r.PathValue("thread"), in.Expected)
	supportReply(w, map[string]bool{"reset": e == nil}, e)
}
func (a *API) intelligenceUsage(w http.ResponseWriter, r *http.Request) {
	v, e := a.IntelligenceService().Usage(r.Context(), user(r), contentScope(r, assistant.ID), r.URL.Query().Get("cursor"))
	supportReply(w, v, e)
}
func (a *API) intelligenceOptions(w http.ResponseWriter, r *http.Request) {
	v, e := a.IntelligenceService().Options(r.Context(), user(r), contentScope(r, workflow.ID))
	supportReply(w, v, e)
}
