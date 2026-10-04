package supportplugins

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/plugins/crm"
	"strconv"
)

func (s *Service) contact(ctx context.Context, tx pgx.Tx, scope sdk.Scope, id string) (crm.Contact, error) {
	var c crm.Contact
	var b []byte
	e := tx.QueryRow(ctx, "SELECT id::text,definition,revision,first_seen,last_seen FROM crm_contacts WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3", scope.AccountID, scope.BotID, id).Scan(&c.ID, &b, &c.Revision, &c.FirstSeen, &c.LastSeen)
	if e != nil {
		return c, e
	}
	var d crm.Contact
	if e = json.Unmarshal(b, &d); e != nil {
		return c, e
	}
	d.ID = c.ID
	d.Revision = c.Revision
	d.FirstSeen = c.FirstSeen
	d.LastSeen = c.LastSeen
	d.Identities = []crm.Identity{}
	rs, e := tx.Query(ctx, "SELECT provider,external_key FROM crm_identities WHERE workspace_id=$1 AND bot_id=$2 AND contact_id=$3 ORDER BY provider,external_key", scope.AccountID, scope.BotID, id)
	if e != nil {
		return c, e
	}
	defer rs.Close()
	for rs.Next() {
		var i crm.Identity
		if e = rs.Scan(&i.Provider, &i.Key); e != nil {
			return c, e
		}
		d.Identities = append(d.Identities, i)
	}
	return d, rs.Err()
}
func (s *Service) resolve(ctx context.Context, tx pgx.Tx, p string, scope sdk.Scope, r crm.ResolveRequest) (crm.Contact, error) {
	if !crm.KeyPattern.MatchString(r.Identity.Provider) || len(r.Identity.Key) == 0 || len(r.Identity.Key) > 128 {
		return crm.Contact{}, database.ErrInvalid
	}
	if r.Identity.Provider == "telegram" {
		n, e := strconv.ParseInt(r.Identity.Key, 10, 64)
		if e != nil || n <= 0 || strconv.FormatInt(n, 10) != r.Identity.Key {
			return crm.Contact{}, database.ErrInvalid
		}
	}
	c := crm.Contact{Name: r.Name, Username: r.Username, Locale: r.Locale, Status: "ACTIVE", Tags: []string{}, Notes: []string{}, Fields: map[string]string{}}
	if !c.Valid() {
		return c, database.ErrInvalid
	}
	var id string
	e := tx.QueryRow(ctx, "SELECT contact_id::text FROM crm_identities WHERE workspace_id=$1 AND bot_id=$2 AND provider=$3 AND external_key=$4", scope.AccountID, scope.BotID, r.Identity.Provider, r.Identity.Key).Scan(&id)
	now := s.now()
	if e == nil {
		_, e = tx.Exec(ctx, "UPDATE crm_contacts SET last_seen=$4,revision=revision+1 WHERE workspace_id=$1 AND bot_id=$2 AND id=$3", scope.AccountID, scope.BotID, id, now)
		if e != nil {
			return c, e
		}
		return s.contact(ctx, tx, scope, id)
	}
	if !noRows(e) {
		return c, e
	}
	count, e := s.count(ctx, tx, scope, "crm.contacts.count", "crm_contacts", "", nil)
	if e != nil {
		return c, e
	}
	if e = s.occupancy(ctx, tx, p, scope, "crm.contacts.count", count+1); e != nil {
		return c, e
	}
	e = tx.QueryRow(ctx, "INSERT INTO crm_contacts(workspace_id,bot_id,definition,first_seen,last_seen) VALUES($1,$2,$3,$4,$4) RETURNING id::text", scope.AccountID, scope.BotID, body(c), now).Scan(&id)
	if e != nil {
		return c, e
	}
	_, e = tx.Exec(ctx, "INSERT INTO crm_identities(workspace_id,bot_id,contact_id,provider,external_key) VALUES($1,$2,$3,$4,$5)", scope.AccountID, scope.BotID, id, r.Identity.Provider, r.Identity.Key)
	if e != nil {
		return c, e
	}
	if e = audit(ctx, tx, p, scope, "crm.contact.created", id, map[string]any{"source": r.Identity.Provider}); e != nil {
		return c, e
	}
	return s.contact(ctx, tx, scope, id)
}
func (s *Service) Resolve(ctx context.Context, actor string, scope sdk.Scope, r crm.ResolveRequest) (crm.Contact, error) {
	scope.PluginID = crm.ID
	var c crm.Contact
	e := s.tenant(ctx, actor, scope, "crm.manage", "crm.write", func(tx pgx.Tx, p string) error { var e error; c, e = s.resolve(ctx, tx, p, scope, r); return e })
	return c, e
}
func (s *Service) Contact(ctx context.Context, actor string, scope sdk.Scope, id string) (crm.Contact, error) {
	scope.PluginID = crm.ID
	var c crm.Contact
	e := s.tenant(ctx, actor, scope, "crm.read", "crm.read", func(tx pgx.Tx, p string) error { var e error; c, e = s.contact(ctx, tx, scope, id); return e })
	return c, e
}
func (s *Service) SaveContact(ctx context.Context, actor string, scope sdk.Scope, id string, c crm.Contact, expected int64) (crm.Contact, error) {
	scope.PluginID = crm.ID
	var out crm.Contact
	if !c.Valid() || expected <= 0 {
		return out, database.ErrInvalid
	}
	d := crm.Contact{Name: c.Name, Username: c.Username, Locale: c.Locale, Status: c.Status, Tags: c.Tags, Notes: c.Notes, Fields: c.Fields}
	e := s.tenant(ctx, actor, scope, "crm.manage", "crm.write", func(tx pgx.Tx, p string) error {
		tag, e := tx.Exec(ctx, "UPDATE crm_contacts SET definition=$4,revision=revision+1 WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3 AND revision=$5", scope.AccountID, scope.BotID, id, body(d), expected)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return database.ErrConflict
		}
		if e = audit(ctx, tx, actor, scope, "crm.contact.updated", id, map[string]any{"revision": expected + 1, "tag_count": len(c.Tags), "note_count": len(c.Notes)}); e != nil {
			return e
		}
		out, e = s.contact(ctx, tx, scope, id)
		return e
	})
	return out, e
}

type Page[T any] struct {
	Items []T    `json:"items"`
	Next  string `json:"next_cursor"`
}
type Filter struct {
	Search     string
	Tag        string
	Status     string
	Priority   string
	Department string
	Assignee   string
	Contact    string
	Cursor     string
	Limit      int
}

func cursor(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	b, e := base64.RawURLEncoding.DecodeString(raw)
	if e != nil || len(b) == 0 || len(b) > 150 {
		return "", database.ErrInvalid
	}
	return string(b), nil
}
func (f Filter) valid() bool {
	return len(f.Search) <= 100 && len(f.Cursor) <= 200 && len(f.Tag) <= 64 && len(f.Status) <= 32 && len(f.Priority) <= 16 && len(f.Department) <= 40 && len(f.Assignee) <= 40 && len(f.Contact) <= 40 && f.Limit > 0 && f.Limit <= 100
}
func (s *Service) Contacts(ctx context.Context, actor string, scope sdk.Scope, f Filter) (Page[crm.Contact], error) {
	scope.PluginID = crm.ID
	out := Page[crm.Contact]{Items: []crm.Contact{}}
	if !f.valid() {
		return out, database.ErrInvalid
	}
	after, e := cursor(f.Cursor)
	if e != nil {
		return out, e
	}
	e = s.tenant(ctx, actor, scope, "crm.read", "crm.read", func(tx pgx.Tx, p string) error {
		rs, e := tx.Query(ctx, `SELECT c.id::text FROM crm_contacts c WHERE c.workspace_id=$1 AND c.bot_id=$2 AND ($3='' OR c.id::text>$3) AND ($4='' OR lower(c.definition->>'name') ^@ lower($4) OR lower(c.definition->>'username') ^@ lower($4) OR EXISTS(SELECT 1 FROM crm_identities i WHERE i.contact_id=c.id AND i.workspace_id=c.workspace_id AND i.bot_id=c.bot_id AND i.external_key ^@ $4)) AND ($5='' OR c.definition->'tags' ? $5) ORDER BY c.id LIMIT $6`, scope.AccountID, scope.BotID, after, f.Search, f.Tag, f.Limit+1)
		if e != nil {
			return e
		}
		ids := []string{}
		for rs.Next() {
			var id string
			if e = rs.Scan(&id); e != nil {
				rs.Close()
				return e
			}
			ids = append(ids, id)
		}
		rs.Close()
		if e = rs.Err(); e != nil {
			return e
		}
		if len(ids) > f.Limit {
			out.Next = base64.RawURLEncoding.EncodeToString([]byte(ids[f.Limit-1]))
			ids = ids[:f.Limit]
		}
		for _, id := range ids {
			c, e := s.contact(ctx, tx, scope, id)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, c)
		}
		return nil
	})
	return out, e
}
func (s *Service) count(ctx context.Context, tx pgx.Tx, scope sdk.Scope, metric, table, condition string, args []any) (int64, error) {
	var license string
	e := tx.QueryRow(ctx, `SELECT definition->'quota'->>'scope' FROM commercial_catalog WHERE kind='QUOTA' AND key=$1 AND effective_at<=$2 ORDER BY version DESC LIMIT 1`, metric, s.now()).Scan(&license)
	if e != nil {
		return 0, e
	}
	bot := scope.BotID
	if license == string(sdk.PerAccount) {
		bot = ""
	}
	var n int64
	values := []any{scope.AccountID, bot}
	values = append(values, args...)
	expr := "count(*)"
	if metric == "ticketing.agents.count" {
		expr = "count(DISTINCT assignee_id)"
	}
	e = tx.QueryRow(ctx, "SELECT "+expr+" FROM "+table+" WHERE workspace_id=$1 AND ($2='' OR bot_id::text=$2) "+condition, values...).Scan(&n)
	return n, e
}
