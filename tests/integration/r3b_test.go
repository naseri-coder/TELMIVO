//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/groupbot"
	"github.com/naseri-coder/TELMIVO/internal/domains/identity"
	"github.com/naseri-coder/TELMIVO/internal/domains/policies"
	"github.com/naseri-coder/TELMIVO/internal/platform/auth"
	"github.com/naseri-coder/TELMIVO/internal/platform/config"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/httpapi"
	"github.com/naseri-coder/TELMIVO/internal/platform/testfixtures"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type r3bAuthority struct {
	mu        sync.Mutex
	admins    map[int64]bool
	fail      bool
	delete    bool
	restrict  bool
	botDelete bool
}

func (a *r3bAuthority) Inspect(_ context.Context, _ int64, user int64) (groupbot.Member, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.fail {
		return groupbot.Member{}, errors.New("unavailable")
	}
	status := "member"
	if a.admins[user] {
		status = "administrator"
	}
	if user == 99997000 {
		status = "creator"
	}
	out := groupbot.Member{Status: status, Delete: a.delete, Restrict: a.restrict, Manage: true}
	out.User.IsBot = user == 99997001
	return out, nil
}
func (a *r3bAuthority) Bot(ctx context.Context, chat int64) (int64, groupbot.Member, error) {
	m, e := a.Inspect(ctx, chat, 99997999)
	m.Status = "administrator"
	m.Delete = a.botDelete
	return 99997999, m, e
}

type groupTransport struct {
	err   error
	calls int
}

func (a *groupTransport) Execute(_ context.Context, _, _, _ int64, _ groupbot.Action, _ int64) error {
	a.calls++
	return a.err
}
func groupService(u identity.Result, tg int64) (*groupbot.Service, *r3bAuthority) {
	a := &r3bAuthority{admins: map[int64]bool{tg: true}, delete: true, restrict: true, botDelete: true}
	return &groupbot.Service{DB: db, Identity: r3aService(), Authority: a, MiniAppURL: "https://example.test"}, a
}
func TestR3BGroupJourney(t *testing.T) {
	s := &groupbot.Service{DB: db, Identity: r3aService()}
	u, e := s.Enter(ctx, auth.TelegramUser{ID: 97010001, FirstName: "R3B"}, "first")
	must(t, e)
	again, e := s.Enter(ctx, auth.TelegramUser{ID: 97010001, FirstName: "R3B"}, "repeat")
	must(t, e)
	if u.UserID != again.UserID || u.WorkspaceID != again.WorkspaceID {
		t.Fatal("entry duplication")
	}
	mini, _ := r3aLogin(t, 97010001, false)
	if mini.UserID != u.UserID || mini.WorkspaceID != u.WorkspaceID {
		t.Fatal("identity mismatch")
	}
	s, a := groupService(u, 97010001)
	var id string
	claim := func() error {
		return db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
			var err error
			id, err = s.Claim(ctx, tx, u.UserID, u.WorkspaceID, "claim", 97010001, -97010001, "گروه آزمون")
			return err
		})
	}
	must(t, claim())
	firstID := id
	must(t, claim())
	if id != firstID {
		t.Fatal("claim duplicate")
	}
	t.Run("trial_once_and_readd", func(t *testing.T) {
		must(t, db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
			var n int
			must(t, tx.QueryRow(ctx, "SELECT count(*) FROM trial_enrollments WHERE scope='PER_GROUP' AND subject_id=$1", id).Scan(&n))
			if n != 1 {
				t.Fatal(n)
			}
			v, e := s.View(ctx, tx, u.UserID, u.WorkspaceID, id)
			if e != nil {
				return e
			}
			if !v.Access.Allowed {
				t.Fatal(v.Access)
			}
			_, e = tx.Exec(ctx, "UPDATE groups SET status='DISCONNECTED' WHERE id=$1", id)
			return e
		}))
		must(t, claim())
	})
	p := groupbot.DefaultPolicy()
	p.Enabled = true
	p.RulesEnabled = true
	p.Rules = []string{"احترام به اعضا"}
	p.DetectionAction = groupbot.Warn
	p.DuplicateCount = 2
	p.WarningThreshold = 2
	p.WarningAction = groupbot.Restrict
	t.Run("configuration_and_conflict", func(t *testing.T) {
		must(t, db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
			_, e := s.Configure(ctx, tx, u.UserID, u.WorkspaceID, id, "config", 1, p)
			return e
		}))
		e := db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
			_, e := s.Configure(ctx, tx, u.UserID, u.WorkspaceID, id, "stale", 1, p)
			return e
		})
		if !errors.Is(e, database.ErrConflict) {
			t.Fatal(e)
		}
	})
	t.Run("duplicate_update_and_warning_escalation", func(t *testing.T) {
		for i := int64(0); i < 3; i++ {
			must(t, db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
				d, e := s.Message(ctx, tx, u.UserID, u.WorkspaceID, id, "message", 97011000+i, groupbot.Message{ID: 1 + i, User: 97019901, Text: "same"})
				if i > 0 && d.Reason != "DUPLICATE_SPAM" {
					t.Fatal(d)
				}
				return e
			}))
		}
		must(t, db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
			d, e := s.Message(ctx, tx, u.UserID, u.WorkspaceID, id, "duplicate", 97011002, groupbot.Message{ID: 3, User: 97019901, Text: "same"})
			if d.Reason != "DUPLICATE_UPDATE" {
				t.Fatal(d)
			}
			var n int
			must(t, tx.QueryRow(ctx, "SELECT count(*) FROM group_moderation_events WHERE group_id=$1 AND kind='WARNING'", id).Scan(&n))
			if n != 2 {
				t.Fatal(n)
			}
			must(t, tx.QueryRow(ctx, "SELECT count(*) FROM group_moderation_events WHERE group_id=$1 AND action='TEMP_RESTRICT' AND outcome='ACTION_REQUESTED'", id).Scan(&n))
			if n != 1 {
				t.Fatal(n)
			}
			return e
		}))
	})
	t.Run("authority_drift_and_unavailable", func(t *testing.T) {
		a.admins[97010001] = false
		e := db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
			_, e := s.Configure(ctx, tx, u.UserID, u.WorkspaceID, id, "revoked", 2, p)
			return e
		})
		if !errors.Is(e, database.ErrForbidden) {
			t.Fatal(e)
		}
		a.admins[97010001] = true
		a.fail = true
		e = db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
			return s.Moderate(ctx, tx, u.UserID, u.WorkspaceID, id, "unavailable", "c", 97019901, 0, groupbot.Ban, "reason")
		})
		if !errors.Is(e, groupbot.ErrAuthority) {
			t.Fatal(e)
		}
		a.fail = false
	})
	t.Run("rights_and_protected_targets", func(t *testing.T) {
		a.botDelete = false
		e := db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
			return s.Moderate(ctx, tx, u.UserID, u.WorkspaceID, id, "no-right", "c", 97019901, 1, groupbot.Delete, "reason")
		})
		if !errors.Is(e, groupbot.ErrRights) {
			t.Fatal(e)
		}
		a.botDelete = true
		for _, target := range []int64{99997000, 99997999, 99997001} {
			e = db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
				return s.Moderate(ctx, tx, u.UserID, u.WorkspaceID, id, fmt.Sprint(target), "c", target, 0, groupbot.Ban, "reason")
			})
			if !errors.Is(e, groupbot.ErrProtected) {
				t.Fatal(e)
			}
		}
	})
	t.Run("transport_results_and_replay", func(t *testing.T) {
		for i, transportErr := range []error{nil, errors.New("rejected"), errors.New("TELEGRAM_DELIVERY_UNKNOWN")} {
			key := fmt.Sprintf("transport-%d", i)
			must(t, db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
				return s.Moderate(ctx, tx, u.UserID, u.WorkspaceID, id, key, "c", 97019901, 0, groupbot.Ban, "reason")
			}))
			tr := &groupTransport{err: transportErr}
			s.Transport = tr
			must(t, db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
				var raw []byte
				must(t, tx.QueryRow(ctx, "SELECT payload FROM outbox_events WHERE event_type='group.action' AND payload->>'operation_key'=$1", key).Scan(&raw))
				if e := s.Deliver(ctx, tx, u.WorkspaceID, "delivery", raw); e != nil {
					return e
				}
				return s.Deliver(ctx, tx, u.WorkspaceID, "retry", raw)
			}))
			if tr.calls != 1 {
				t.Fatal(tr.calls)
			}
			must(t, db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
				var got string
				must(t, tx.QueryRow(ctx, "SELECT outcome FROM group_moderation_events WHERE group_id=$1 AND operation_key=$2 AND kind='ACTION_RESULT'", id, key).Scan(&got))
				want := []string{"ACTION_ACKNOWLEDGED", "ACTION_FAILED", "ACTION_OUTCOME_UNKNOWN"}[i]
				if got != want {
					t.Fatal(got)
				}
				return nil
			}))
		}
	})
	t.Run("nonadmin_claim_denied", func(t *testing.T) {
		a.admins[97010001] = false
		e := db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
			_, e := s.Claim(ctx, tx, u.UserID, u.WorkspaceID, "denied", 97010001, -97010100, "denied")
			return e
		})
		if !errors.Is(e, database.ErrForbidden) {
			t.Fatal(e)
		}
		a.admins[97010001] = true
	})
	t.Run("expiry_preserves_rules", func(t *testing.T) {
		must(t, db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
			v, e := s.View(ctx, tx, u.UserID, u.WorkspaceID, id)
			if e != nil {
				return e
			}
			if len(v.Policy.Rules) != 1 {
				t.Fatal(v)
			}
			var end time.Time
			must(t, tx.QueryRow(ctx, "SELECT ends_at FROM trial_enrollments WHERE scope='PER_GROUP' AND subject_id=$1", id).Scan(&end))
			var role string
			must(t, tx.QueryRow(ctx, "SELECT role_key FROM workspace_members WHERE workspace_id=$1 AND user_id=$2", u.WorkspaceID, u.UserID).Scan(&role))
			d, e := policies.Resolve(ctx, tx, policies.Request{User: u.UserID, Workspace: u.WorkspaceID, Role: role, Feature: "groups", Permission: "groups.moderate", Group: id, Now: end.Add(24 * time.Hour)})
			if e != nil {
				return e
			}
			if d.Allowed {
				t.Fatal("expired access allowed", d)
			}
			s.Now = func() time.Time { return end.Add(24 * time.Hour) }
			got, err := s.Message(ctx, tx, u.UserID, u.WorkspaceID, id, "expired", 97011999, groupbot.Message{ID: 9, User: 97019901, Text: "same"})
			if err != nil {
				return err
			}
			if got.Reason != "ACCESS_DISABLED" {
				t.Fatal(got)
			}
			return nil
		}))
	})
}
func TestR3BConcurrencyIsolation(t *testing.T) {
	u, _ := r3aLogin(t, 97020001, false)
	v, _ := r3aLogin(t, 97020002, false)
	s, a := groupService(u, 97020001)
	a.admins[97020002] = true
	var wg sync.WaitGroup
	results := make(chan error, 2)
	ids := make(chan string, 2)
	for _, item := range []struct {
		u  identity.Result
		tg int64
	}{{u, 97020001}, {v, 97020002}} {
		wg.Add(1)
		go func(x struct {
			u  identity.Result
			tg int64
		}) {
			defer wg.Done()
			results <- db.Tenant(ctx, x.u.UserID, x.u.WorkspaceID, false, func(tx pgx.Tx) error {
				id, e := s.Claim(ctx, tx, x.u.UserID, x.u.WorkspaceID, "race", x.tg, -97020000, "race")
				if e == nil {
					ids <- id
				}
				return e
			})
		}(item)
	}
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		} else if e.Error() == "" {
			t.Fatal(e)
		}
	}
	if success != 1 {
		t.Fatal(success)
	}
	id := <-ids
	var ws string
	must(t, db.Pool.QueryRow(ctx, "SELECT workspace_id::text FROM telegram_group_route($1)", -97020000).Scan(&ws))
	winner := u
	loser := v
	if ws == v.WorkspaceID {
		winner = v
		loser = u
	}
	e := db.Tenant(ctx, loser.UserID, loser.WorkspaceID, false, func(tx pgx.Tx) error { _, e := s.View(ctx, tx, loser.UserID, loser.WorkspaceID, id); return e })
	if e == nil {
		t.Fatal("isolation")
	}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e := db.Tenant(ctx, winner.UserID, ws, false, func(tx pgx.Tx) error {
				return s.Moderate(ctx, tx, winner.UserID, ws, id, "same-warning", "c", 97029999, 0, groupbot.Warn, "reason")
			})
			if e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	must(t, db.Tenant(ctx, winner.UserID, ws, false, func(tx pgx.Tx) error {
		var n int
		must(t, tx.QueryRow(ctx, "SELECT count(*) FROM group_moderation_events WHERE group_id=$1 AND kind='WARNING'", id).Scan(&n))
		if n != 1 {
			t.Fatal(n)
		}
		return nil
	}))
}
func TestR3BBrowserFixture(t *testing.T) {
	u, token := r3aLogin(t, 97000001, false)
	other := r3aExtraWorkspace(t, u, "فضای بدون گروه R3B", "OWNER")
	_ = other
	s := &groupbot.Service{DB: db, Identity: r3aService(), Authority: testfixtures.GroupAuthority{}}
	var id string
	must(t, db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
		var e error
		id, e = s.Claim(ctx, tx, u.UserID, u.WorkspaceID, "browser", 97000001, -97000001, "گروه فارسی R3B")
		return e
	}))
	must(t, db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
		return s.Moderate(ctx, tx, u.UserID, u.WorkspaceID, id, "fixture-warning", "c", 97000123, 0, groupbot.Warn, "هشدار آزمایشی")
	}))
	api := &httpapi.API{DB: db, Identity: r3aService(), Groups: s, Config: config.Config{Environment: "test", Synthetic: true, MiniAppURL: "http://127.0.0.1:5173"}}
	for _, path := range []string{"/api/v1/groups", "/api/v1/groups/" + id, "/api/v1/groups/" + id + "/history"} {
		rec := r3aRequest(api.Handler(), token, u.WorkspaceID, http.MethodGet, path, nil)
		if rec.Code != 200 {
			t.Fatal(rec.Code, rec.Body.String())
		}
		var out any
		must(t, json.Unmarshal(rec.Body.Bytes(), &out))
	}
}

func TestR3BDeterministicNative(t *testing.T) {
	u, _ := r3aLogin(t, 97040001, false)
	s, _ := groupService(u, 97040001)
	var id string
	must(t, db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
		var e error
		id, e = s.Claim(ctx, tx, u.UserID, u.WorkspaceID, "claim", 97040001, -97040001, "سیاست‌ها")
		return e
	}))
	p := groupbot.DefaultPolicy()
	p.Enabled = true
	p.FloodCount = 2
	p.DetectionAction = groupbot.Warn
	rev := int64(1)
	config := func() {
		must(t, db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
			_, e := s.Configure(ctx, tx, u.UserID, u.WorkspaceID, id, "configuration", rev, p)
			return e
		}))
		rev++
	}
	config()
	message := func(update, target int64, text, reason string) {
		must(t, db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
			d, e := s.Message(ctx, tx, u.UserID, u.WorkspaceID, id, "decision", update, groupbot.Message{ID: update, User: target, Text: text})
			if e == nil && d.Reason != reason {
				t.Fatal(d, reason)
			}
			return e
		}))
	}
	t.Run("flood_and_configurable_threshold", func(t *testing.T) {
		message(97040010, 97049901, "a", "NO_MATCH")
		message(97040011, 97049901, "b", "FLOOD")
		p.FloodCount = 4
		config()
		message(97040012, 97049901, "c", "NO_MATCH")
	})
	t.Run("link_allowlist_and_denylist", func(t *testing.T) {
		p.FloodCount = 0
		p.LinkMode = "ALLOWLIST"
		p.Domains = []string{"example.com"}
		config()
		message(97040020, 97049902, "https://example.com/a", "NO_MATCH")
		message(97040021, 97049902, "https://example.com.attacker.test", "LINK_POLICY")
		p.LinkMode = "DENYLIST"
		config()
		message(97040022, 97049902, "https://sub.example.com/a", "LINK_POLICY")
		message(97040023, 97049902, "https://other.test", "NO_MATCH")
	})
	t.Run("concurrent_update_dedup", func(t *testing.T) {
		p.LinkMode = "ALLOW_ALL"
		p.DuplicateCount = 1
		config()
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				e := db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
					_, e := s.Message(ctx, tx, u.UserID, u.WorkspaceID, id, "race", 97040030, groupbot.Message{ID: 30, User: 97049903, Text: "same"})
					return e
				})
				if e != nil {
					t.Error(e)
				}
			}()
		}
		wg.Wait()
		must(t, db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
			var n int
			must(t, tx.QueryRow(ctx, "SELECT count(*) FROM group_moderation_events WHERE group_id=$1 AND operation_key='update:97040030' AND kind='WARNING'", id).Scan(&n))
			if n != 1 {
				t.Fatal(n)
			}
			return nil
		}))
	})
	t.Run("concurrent_configuration_conflict", func(t *testing.T) {
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results <- db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
					_, e := s.Configure(ctx, tx, u.UserID, u.WorkspaceID, id, "race-config", rev, p)
					return e
				})
			}()
		}
		wg.Wait()
		close(results)
		pass, conflict := 0, 0
		for e := range results {
			if e == nil {
				pass++
			} else if errors.Is(e, database.ErrConflict) {
				conflict++
			} else {
				t.Fatal(e)
			}
		}
		if pass != 1 || conflict != 1 {
			t.Fatal(pass, conflict)
		}
	})
	t.Run("clear_history_and_key_conflict", func(t *testing.T) {
		must(t, db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
			return s.Moderate(ctx, tx, u.UserID, u.WorkspaceID, id, "clear", "c", 97049903, 0, groupbot.Clear, "clear")
		}))
		e := db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
			return s.Moderate(ctx, tx, u.UserID, u.WorkspaceID, id, "clear", "c", 97049904, 0, groupbot.Clear, "different target")
		})
		if !errors.Is(e, database.ErrConflict) {
			t.Fatal(e)
		}
		must(t, db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
			var n int
			must(t, tx.QueryRow(ctx, "SELECT count(*) FROM group_moderation_events WHERE group_id=$1 AND kind='WARNING'", id).Scan(&n))
			if n < 1 {
				t.Fatal("history erased")
			}
			return nil
		}))
	})
}
func TestR3BTelegramFixtures(t *testing.T) {
	u, _ := r3aLogin(t, 97030001, false)
	s, _ := groupService(u, 97030001)
	handle := func(raw string) {
		var update groupbot.Update
		must(t, json.Unmarshal([]byte(raw), &update))
		must(t, s.Handle(ctx, update, "fixture"))
	}
	handle(`{"update_id":97030000,"my_chat_member":{"chat":{"id":-97030001,"type":"supergroup","title":"نصب"},"from":{"id":97030001,"first_name":"installer"},"new_chat_member":{"status":"administrator"}}}`)
	var id string
	must(t, db.Pool.QueryRow(ctx, "SELECT group_id::text FROM telegram_group_route($1)", -97030001).Scan(&id))
	for i, cmd := range []string{"/status", "/rules", "/settings", "/warnings", "/help"} {
		handle(fmt.Sprintf(`{"update_id":%d,"message":{"message_id":1,"from":{"id":97030001},"chat":{"id":-97030001,"type":"supergroup"},"text":%q}}`, 97030100+i, cmd))
	}
	raw := `{"update_id":97030200,"message":{"message_id":2,"from":{"id":97030001},"chat":{"id":-97030001,"type":"supergroup"},"text":"/warn آزمایشی","reply_to_message":{"message_id":1,"from":{"id":97039999}}}}`
	handle(raw)
	handle(raw)
	must(t, db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
		var n int
		must(t, tx.QueryRow(ctx, "SELECT count(*) FROM group_moderation_events WHERE group_id=$1 AND kind='WARNING'", id).Scan(&n))
		if n != 1 {
			t.Fatal(n)
		}
		return nil
	}))
	handle(`{"update_id":97030300,"my_chat_member":{"chat":{"id":-97030001,"type":"supergroup","title":"نصب"},"from":{"id":97030001},"new_chat_member":{"status":"left"}}}`)
	handle(`{"update_id":97030301,"my_chat_member":{"chat":{"id":-97030001,"type":"supergroup","title":"نصب"},"from":{"id":97030001},"new_chat_member":{"status":"administrator"}}}`)
	must(t, db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
		var n int
		must(t, tx.QueryRow(ctx, "SELECT count(*) FROM trial_enrollments WHERE scope='PER_GROUP' AND subject_id=$1", id).Scan(&n))
		if n != 1 {
			t.Fatal("restart", n)
		}
		return nil
	}))
	handle(`{"update_id":97030400,"message":{"message_id":4,"from":{"id":97030001},"chat":{"id":-97030001,"type":"supergroup"},"migrate_to_chat_id":-10097030001}}`)
	handle(`{"update_id":97030401,"message":{"message_id":5,"from":{"id":97030001},"chat":{"id":-10097030001,"type":"supergroup"},"migrate_from_chat_id":-97030001}}`)
	var migrated string
	must(t, db.Pool.QueryRow(ctx, "SELECT group_id::text FROM telegram_group_route($1)", -10097030001).Scan(&migrated))
	if migrated != id {
		t.Fatal("migration changed identity")
	}
	if s.Handle(ctx, groupbot.Update{}, "malformed") == nil {
		t.Fatal("malformed accepted")
	}
}
func TestR3BAPIIsolationAuthority(t *testing.T) {
	u, token := r3aLogin(t, 97050001, false)
	v, vtoken := r3aLogin(t, 97050002, true)
	s, a := groupService(u, 97050001)
	a.admins[97050002] = true
	api := &httpapi.API{DB: db, Identity: r3aService(), Groups: s, Config: config.Config{Environment: "test", Synthetic: true, MiniAppURL: "https://mini.test"}}
	h := api.Handler()
	rec := r3aRequest(h, token, u.WorkspaceID, "POST", "/api/v1/groups/claims", map[string]any{"chat_id": -97050001, "title": "owned"})
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var out map[string]string
	must(t, json.Unmarshal(rec.Body.Bytes(), &out))
	id := out["id"]
	t.Run("cross_workspace_conflict_audited", func(t *testing.T) {
		rec := r3aRequest(h, vtoken, v.WorkspaceID, "POST", "/api/v1/groups/claims", map[string]any{"chat_id": -97050001, "title": "steal"})
		if rec.Code != 409 {
			t.Fatal(rec.Code, rec.Body.String())
		}
		must(t, db.Tenant(ctx, v.UserID, v.WorkspaceID, false, func(tx pgx.Tx) error {
			var n int
			must(t, tx.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE workspace_id=$1 AND action='group.claim.conflict'", v.WorkspaceID).Scan(&n))
			if n != 1 {
				t.Fatal(n)
			}
			return nil
		}))
	})
	t.Run("platform_admin_not_group_admin", func(t *testing.T) {
		a.admins[97050002] = false
		rec := r3aRequest(h, vtoken, v.WorkspaceID, "POST", "/api/v1/groups/claims", map[string]any{"chat_id": -97050002, "title": "denied"})
		if rec.Code != 403 {
			t.Fatal(rec.Code)
		}
		a.admins[97050002] = true
	})
	t.Run("cross_workspace_reads_and_mutations", func(t *testing.T) {
		for _, path := range []string{"/api/v1/groups/" + id, "/api/v1/groups/" + id + "/history"} {
			rec := r3aRequest(h, vtoken, v.WorkspaceID, "GET", path, nil)
			if rec.Code == 200 {
				t.Fatal("foreign read")
			}
		}
		p := groupbot.DefaultPolicy()
		rec := r3aRequest(h, vtoken, v.WorkspaceID, "PATCH", "/api/v1/groups/"+id+"/policy", map[string]any{"revision": 1, "policy": p})
		if rec.Code == 200 {
			t.Fatal("foreign mutation")
		}
		rec = r3aRequest(h, vtoken, v.WorkspaceID, "GET", "/api/v1/groups", nil)
		if strings.TrimSpace(rec.Body.String()) != "[]" {
			t.Fatal(rec.Body.String())
		}
	})
	t.Run("application_permission_separate", func(t *testing.T) {
		must(t, db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, "INSERT INTO workspace_members(workspace_id,user_id,role_key) VALUES($1,$2,'ANALYST')", u.WorkspaceID, v.UserID)
			return e
		}))
		rec := r3aRequest(h, vtoken, u.WorkspaceID, "POST", "/api/v1/groups/"+id+"/actions", map[string]any{"target_id": 97059999, "action": "BAN", "reason": "denied", "idempotency_key": "viewer"})
		if rec.Code != 403 {
			t.Fatal(rec.Code, rec.Body.String())
		}
	})
}
func TestR3BPrivateStartAndHiddenLink(t *testing.T) {
	s := &groupbot.Service{DB: db, Identity: r3aService(), MiniAppURL: "https://mini.test"}
	handle := func(tg, update int64) {
		var u groupbot.Update
		must(t, json.Unmarshal([]byte(fmt.Sprintf(`{"update_id":%d,"message":{"from":{"id":%d,"first_name":"Group First"},"chat":{"id":%d,"type":"private"},"text":"/start"}}`, update, tg, tg)), &u))
		must(t, s.Handle(ctx, u, "private"))
	}
	handle(97060001, 97060001)
	handle(97060001, 97060001)
	handle(97060001, 97060002)
	u, _ := r3aLogin(t, 97060001, false)
	must(t, db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
		var n int
		must(t, tx.QueryRow(ctx, "SELECT count(*) FROM telegram_accounts WHERE telegram_id=97060001").Scan(&n))
		if n != 1 {
			t.Fatal(n)
		}
		must(t, tx.QueryRow(ctx, "SELECT count(*) FROM workspaces WHERE owner_id=$1", u.UserID).Scan(&n))
		if n != 1 {
			t.Fatal(n)
		}
		must(t, tx.QueryRow(ctx, "SELECT count(*) FROM identity_entries WHERE user_id=$1 AND source='GROUP_BOT'", u.UserID).Scan(&n))
		if n != 1 {
			t.Fatal(n)
		}
		return nil
	}))
	mini, _ := r3aLogin(t, 97060002, false)
	handle(97060002, 97060003)
	after, e := s.Enter(ctx, auth.TelegramUser{ID: 97060002, FirstName: "known"}, "after-group")
	must(t, e)
	if mini.UserID != after.UserID || mini.WorkspaceID != after.WorkspaceID {
		t.Fatal("entry mismatch")
	}
	s, _ = groupService(u, 97060001)
	var id string
	must(t, db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
		var e error
		id, e = s.Claim(ctx, tx, u.UserID, u.WorkspaceID, "claim", 97060001, -97060001, "Hidden links")
		return e
	}))
	p := groupbot.DefaultPolicy()
	p.Enabled = true
	p.LinkMode = "DENY_EXTERNAL"
	p.DetectionAction = groupbot.Warn
	must(t, db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
		_, e := s.Configure(ctx, tx, u.UserID, u.WorkspaceID, id, "config", 1, p)
		return e
	}))
	var update groupbot.Update
	must(t, json.Unmarshal([]byte(`{"update_id":97060020,"message":{"message_id":20,"from":{"id":97069999},"chat":{"id":-97060001,"type":"supergroup"},"text":"click","entities":[{"type":"text_link","url":"https://attacker.test"}]}}`), &update))
	must(t, s.Handle(ctx, update, "hidden"))
	must(t, db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
		var n int
		must(t, tx.QueryRow(ctx, "SELECT count(*) FROM group_moderation_events WHERE group_id=$1 AND reason='LINK_POLICY' AND kind='WARNING'", id).Scan(&n))
		if n != 1 {
			t.Fatal(n)
		}
		return nil
	}))
	e = db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
		return s.Moderate(ctx, tx, u.UserID, u.WorkspaceID, id, "update:97060020", "reuse", 97069999, 20, groupbot.Ban, "different action")
	})
	if !errors.Is(e, database.ErrConflict) {
		t.Fatal(e)
	}
}
