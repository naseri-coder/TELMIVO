package businessplugins

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/plugins/crm"
	"github.com/naseri-coder/TELMIVO/internal/plugins/forms"
	"strconv"
)

func fmtUser(n int64) string { return strconv.FormatInt(n, 10) }
func (s *Service) SaveForm(ctx context.Context, actor string, scope sdk.Scope, id string, in Change[forms.Form]) (Definition, error) {
	scope.PluginID = forms.ID
	f := in.Definition
	if !f.Valid() {
		return Definition{}, database.ErrInvalid
	}
	f.ID = ""
	f.Revision = 0
	return s.save(ctx, actor, scope, id, f.Name, in.Key, in.Expected, f)
}

// Customer is constructed only by the authenticated per-bot Telegram webhook.
// Never bind these fields to an owner-facing HTTP body or model tool.
type Customer struct {
	User     int64
	Name     string
	Username string
	Locale   string
}
type Submission struct {
	ID       string                     `json:"id"`
	Form     string                     `json:"form"`
	Contact  string                     `json:"contact"`
	Revision int64                      `json:"form_revision"`
	Snapshot json.RawMessage            `json:"snapshot"`
	Answers  map[string]json.RawMessage `json:"answers"`
}

func (s *Service) SubmitTrusted(ctx context.Context, actor string, scope sdk.Scope, id, k string, c Customer, values map[string]json.RawMessage) (Submission, error) {
	scope.PluginID = forms.ID
	var out Submission
	if c.User <= 0 || !key(k) || len(body(values)) > 8192 {
		return out, database.ErrInvalid
	}
	fp := hash([]any{"submission", actor, id, c, values})
	var f forms.Form
	var expected int64
	var replay bool
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		b, exists, e := receipt(ctx, tx, scope, k, fp)
		if e != nil {
			return e
		}
		if exists {
			replay = true
			return json.Unmarshal(b, &out)
		}
		d, e := get(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		expected = d.Revision
		return json.Unmarshal(d.Data, &f)
	})
	if e != nil || replay {
		return out, e
	}
	fields, tags, valid := f.Validate(values)
	if !valid || !f.Enabled {
		return out, database.ErrInvalid
	}
	if f.Access == "ACCESS_POLICY_PROTECTED" {
		if s.Content == nil {
			return out, database.ErrForbidden
		}
		d, e := s.Content.BusinessAccess(ctx, actor, scope, f.Policy, c.User)
		if e != nil {
			return out, e
		}
		if !d.Allowed {
			return out, database.ErrForbidden
		}
	}
	e = s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		b, exists, e := receipt(ctx, tx, scope, k, fp)
		if e != nil {
			return e
		}
		if exists {
			return json.Unmarshal(b, &out)
		}
		if e = s.admit(ctx, tx, actor, scope, "forms.submit", true); e != nil {
			return e
		}
		d, e := get(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		if d.Revision != expected {
			return database.ErrConflict
		}
		if e = s.crm(ctx, tx, scope, true); e != nil {
			return e
		}
		if e = s.quota(ctx, tx, scope, "forms.submissions.monthly", 1, true); e != nil {
			return e
		}
		p, e := s.principal(ctx, tx, scope)
		if e != nil {
			return e
		}
		contact, e := s.Support.ResolveBusinessTx(ctx, tx, p, scope, crm.ResolveRequest{Identity: crm.Identity{Provider: "telegram", Key: fmtUser(c.User)}, Name: c.Name, Username: c.Username, Locale: c.Locale}, fields, tags)
		if e != nil {
			return e
		}
		out = Submission{Form: id, Contact: contact.ID, Revision: expected, Snapshot: d.Data, Answers: values}
		e = tx.QueryRow(ctx, "INSERT INTO form_submissions(workspace_id,bot_id,form_id,contact_id,form_revision,snapshot,answers,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text", scope.AccountID, scope.BotID, id, contact.ID, expected, d.Data, body(values), s.now()).Scan(&out.ID)
		if e != nil {
			return e
		}
		if e = emit(ctx, tx, actor, scope, "form.submitted", out.ID, contact.ID, map[string][]string{"form.id": {id}}); e != nil {
			return e
		}
		return record(ctx, tx, scope, k, fp, out)
	})
	return out, e
}

type SubmissionPage struct {
	Items []Submission `json:"items"`
	Next  string       `json:"next_cursor"`
}

func (s *Service) Submissions(ctx context.Context, actor string, scope sdk.Scope, id, after string) (SubmissionPage, error) {
	scope.PluginID = forms.ID
	out := SubmissionPage{Items: []Submission{}}
	if len(after) > 36 {
		return out, database.ErrInvalid
	}
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		rs, e := tx.Query(ctx, "SELECT id::text,form_id::text,contact_id::text,form_revision,snapshot,answers FROM form_submissions WHERE workspace_id=$1 AND bot_id=$2 AND form_id::text=$3 AND ($4='' OR id::text>$4) ORDER BY id LIMIT 51", scope.AccountID, scope.BotID, id, after)
		if e != nil {
			return e
		}
		defer rs.Close()
		for rs.Next() {
			var v Submission
			var b []byte
			if e = rs.Scan(&v.ID, &v.Form, &v.Contact, &v.Revision, &v.Snapshot, &b); e != nil {
				return e
			}
			if e = json.Unmarshal(b, &v.Answers); e != nil {
				return e
			}
			out.Items = append(out.Items, v)
		}
		if len(out.Items) > 50 {
			out.Items = out.Items[:50]
			out.Next = out.Items[49].ID
		}
		return rs.Err()
	})
	return out, e
}
