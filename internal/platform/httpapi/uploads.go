package httpapi

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	"github.com/naseri-coder/TELMIVO/internal/domains/uploads"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	files "github.com/naseri-coder/TELMIVO/internal/plugins/filemanager"
)

func (a *API) uploader() *uploads.Service {
	if a.Uploads != nil {
		return a.Uploads
	}
	if a.Storage == nil {
		return &uploads.Service{DB: a.DB}
	}
	return &uploads.Service{DB: a.DB, Storage: *a.Storage}
}

func (a *API) uploadWorkflows(w http.ResponseWriter, r *http.Request) {
	out, e := a.uploader().Workflows(r.Context(), user(r), workspace(r))
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}

func (a *API) uploadWorkflowSave(w http.ResponseWriter, r *http.Request) {
	var in uploads.WorkflowChange
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	out, e := a.uploader().SaveWorkflow(r.Context(), user(r), workspace(r), r.PathValue("workflow"), correlation(r), in)
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}

func (a *API) uploadList(w http.ResponseWriter, r *http.Request) {
	out, e := a.uploader().List(r.Context(), user(r), workspace(r))
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}

func (a *API) uploadCreate(w http.ResponseWriter, r *http.Request) {
	var in uploads.Create
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	out, e := a.uploader().Create(r.Context(), user(r), workspace(r), correlation(r), in)
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}

func (a *API) uploadContent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("upload")
	item, e := a.uploader().Detail(r.Context(), user(r), workspace(r), id)
	if e != nil {
		handleError(w, e)
		return
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
	if r.ContentLength <= 0 || r.ContentLength != item.SizeBytes || contentType != item.ContentType {
		fail(w, 400, "invalid")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, item.SizeBytes+1)
	out, e := a.uploader().PutContent(r.Context(), user(r), workspace(r), id, correlation(r), r.Body, r.ContentLength)
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}

func (a *API) uploadAction(w http.ResponseWriter, r *http.Request) {
	var in uploads.Action
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	out, e := a.uploader().Action(r.Context(), user(r), workspace(r), r.PathValue("upload"), correlation(r), in)
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}

func (a *API) uploadOpen(w http.ResponseWriter, r *http.Request) {
	reader, meta, e := a.uploader().Open(r.Context(), user(r), workspace(r), r.PathValue("upload"))
	if e != nil {
		handleError(w, e)
		return
	}
	defer reader.Close()
	w.Header().Set("Content-Type", meta.ContentType)
	w.Header().Set("Content-Length", intString(meta.Size))
	w.Header().Set("Content-Disposition", "attachment")
	w.WriteHeader(200)
	_, _ = io.Copy(w, reader)
}

func intString(n int64) string {
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

func (a *API) uploadSignedURL(w http.ResponseWriter, r *http.Request) {
	raw, e := a.uploader().SignedURL(r.Context(), user(r), workspace(r), r.PathValue("upload"), 10*time.Minute)
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, map[string]string{"url": raw})
}

func (a *API) uploadHandoff(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Bot    string `json:"bot_id"`
		Title  string `json:"title"`
		Policy string `json:"policy,omitempty"`
		Key    string `json:"idempotency_key"`
	}
	if decode(w, r, &in) != nil || in.Bot == "" || strings.TrimSpace(in.Title) == "" || len(in.Title) > 200 {
		fail(w, 400, "invalid")
		return
	}
	item, e := a.uploader().Detail(r.Context(), user(r), workspace(r), r.PathValue("upload"))
	if e != nil {
		handleError(w, e)
		return
	}
	if item.State != "STORED" || (item.CustomerBotID != "" && item.CustomerBotID != in.Bot) {
		handleError(w, database.ErrForbidden)
		return
	}
	kind := "document"
	switch {
	case strings.HasPrefix(item.ContentType, "image/"):
		kind = "photo"
	case strings.HasPrefix(item.ContentType, "video/"):
		kind = "video"
	case strings.HasPrefix(item.ContentType, "audio/"):
		kind = "audio"
	}
	scope := sdk.Scope{AccountID: workspace(r), BotID: in.Bot, PluginID: files.ID}
	out, e := a.content().SaveFile(r.Context(), user(r), scope, "", correlation(r), contentplugins.FileChange{
		File: files.File{FileID: "upload_" + item.ID, UniqueID: "upload_" + item.ID, Kind: kind, Title: strings.TrimSpace(in.Title), Policy: in.Policy, Enabled: true},
		Key:  in.Key,
	})
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
