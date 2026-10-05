package appleservices

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/entitlements"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	plugin "github.com/naseri-coder/TELMIVO/internal/plugins/appleservices"
)

type Service struct {
	DB    *database.Store
	Clock func() time.Time
}

func (s *Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

func validCorrelation(v string) bool {
	return v != "" && len(v) <= 150 && strings.TrimSpace(v) == v
}

func validKey(v string) bool {
	return v != "" && len(v) <= 100 && strings.TrimSpace(v) == v
}

func fingerprint(in CreateRequest) string {
	expiry := ""
	if in.ExpiresAt != nil {
		expiry = in.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	sum := sha256.Sum256([]byte(string(in.Kind) + "\x00" + in.SubjectRef + "\x00" + expiry))
	return hex.EncodeToString(sum[:])
}

func (s *Service) profile(ctx context.Context, tx pgx.Tx, workspace string) (Profile, error) {
	var out Profile
	err := tx.QueryRow(ctx, `SELECT workspace_id::text,state,revision,created_at,updated_at
		FROM apple_service_profiles WHERE workspace_id=$1`, workspace).
		Scan(&out.Workspace, &out.State, &out.Revision, &out.Created, &out.Updated)
	return out, err
}

func (s *Service) ensureProfileTx(ctx context.Context, tx pgx.Tx, actor, workspace string) (Profile, bool, error) {
	out, err := s.profile(ctx, tx, workspace)
	if err == nil {
		return out, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO apple_service_profiles(workspace_id,created_by,created_at,updated_at)
		VALUES($1,$2,$3,$3)`, workspace, actor, s.now())
	if err != nil {
		return Profile{}, false, err
	}
	out, err = s.profile(ctx, tx, workspace)
	return out, true, err
}

func (s *Service) Profile(ctx context.Context, actor, workspace string) (Profile, error) {
	var out Profile
	if s.DB == nil {
		return out, database.ErrInvalid
	}
	err := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		if err := entitlements.Require(ctx, tx, actor, workspace, plugin.Feature, plugin.PermissionRead, 0); err != nil {
			return err
		}
		var err error
		out, err = s.profile(ctx, tx, workspace)
		return err
	})
	return out, err
}

func (s *Service) EnsureProfile(ctx context.Context, actor, workspace, correlation string) (Profile, error) {
	var out Profile
	if s.DB == nil || !validCorrelation(correlation) {
		return out, database.ErrInvalid
	}
	err := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		if err := entitlements.Require(ctx, tx, actor, workspace, plugin.Feature, plugin.PermissionManage, 0); err != nil {
			return err
		}
		var created bool
		var err error
		out, created, err = s.ensureProfileTx(ctx, tx, actor, workspace)
		if err != nil {
			return err
		}
		if !created {
			return nil
		}
		return database.Audit(ctx, tx, actor, workspace, "apple.profile.created", "apple_service_profile", workspace, correlation, map[string]any{}, map[string]any{"state": out.State, "revision": out.Revision})
	})
	return out, err
}

func (s *Service) SetProfileState(ctx context.Context, actor, workspace, correlation string, in ProfileAction) (Profile, error) {
	var out Profile
	in.State = strings.TrimSpace(in.State)
	in.Reason = strings.TrimSpace(in.Reason)
	if s.DB == nil || !validCorrelation(correlation) || in.Expected < 1 || (in.State != "FOUNDATION_READY" && in.State != "SUSPENDED") || in.Reason == "" || len(in.Reason) > 500 {
		return out, database.ErrInvalid
	}
	err := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		if err := entitlements.Require(ctx, tx, actor, workspace, plugin.Feature, plugin.PermissionManage, 0); err != nil {
			return err
		}
		var current Profile
		if err := tx.QueryRow(ctx, `SELECT workspace_id::text,state,revision,created_at,updated_at
			FROM apple_service_profiles WHERE workspace_id=$1 FOR UPDATE`, workspace).
			Scan(&current.Workspace, &current.State, &current.Revision, &current.Created, &current.Updated); err != nil {
			return err
		}
		if current.Revision != in.Expected {
			return database.ErrConflict
		}
		if current.State == in.State {
			out = current
			return nil
		}
		if current.State != "FOUNDATION_READY" && current.State != "SUSPENDED" {
			return database.ErrConflict
		}
		tag, err := tx.Exec(ctx, `UPDATE apple_service_profiles
			SET state=$2,revision=revision+1,updated_at=$3
			WHERE workspace_id=$1 AND revision=$4`, workspace, in.State, s.now(), in.Expected)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return database.ErrConflict
		}
		out, err = s.profile(ctx, tx, workspace)
		if err != nil {
			return err
		}
		return database.Audit(ctx, tx, actor, workspace, "apple.profile.state_changed", "apple_service_profile", workspace, correlation, map[string]any{"state": current.State, "revision": current.Revision}, map[string]any{"state": out.State, "revision": out.Revision, "reason": in.Reason})
	})
	return out, err
}

func (s *Service) operation(ctx context.Context, tx pgx.Tx, workspace, id string, lock bool) (Operation, error) {
	var out Operation
	query := `SELECT id::text,kind,state,subject_ref,revision,expires_at,created_at,updated_at
		FROM apple_service_operations WHERE workspace_id=$1 AND id::text=$2`
	if lock {
		query += " FOR UPDATE"
	}
	err := tx.QueryRow(ctx, query, workspace, id).Scan(&out.ID, &out.Kind, &out.State, &out.SubjectRef, &out.Revision, &out.ExpiresAt, &out.Created, &out.Updated)
	return out, err
}

func (s *Service) operationByKey(ctx context.Context, tx pgx.Tx, workspace, key string) (Operation, string, error) {
	var out Operation
	var savedFingerprint string
	err := tx.QueryRow(ctx, `SELECT id::text,kind,state,subject_ref,revision,expires_at,created_at,updated_at,fingerprint
		FROM apple_service_operations WHERE workspace_id=$1 AND operation_key=$2`, workspace, key).
		Scan(&out.ID, &out.Kind, &out.State, &out.SubjectRef, &out.Revision, &out.ExpiresAt, &out.Created, &out.Updated, &savedFingerprint)
	return out, savedFingerprint, err
}

func (s *Service) CreateOperation(ctx context.Context, actor, workspace, correlation string, in CreateRequest) (Operation, error) {
	var out Operation
	in.SubjectRef = strings.TrimSpace(in.SubjectRef)
	in.Key = strings.TrimSpace(in.Key)
	if s.DB == nil || !in.Kind.Valid() || !validKey(in.Key) || !validCorrelation(correlation) || len(in.SubjectRef) > 200 {
		return out, database.ErrInvalid
	}
	if in.ExpiresAt != nil && !in.ExpiresAt.After(s.now()) {
		return out, database.ErrInvalid
	}
	capability := Capability(in.Kind)
	if capability == "" {
		return out, database.ErrInvalid
	}
	fp := fingerprint(in)
	err := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		if err := entitlements.Require(ctx, tx, actor, workspace, plugin.Feature, capability, 0); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,7500240))", workspace+":"+in.Key); err != nil {
			return err
		}
		replay, saved, err := s.operationByKey(ctx, tx, workspace, in.Key)
		if err == nil {
			if saved != fp {
				return database.ErrConflict
			}
			out = replay
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		profile, createdProfile, err := s.ensureProfileTx(ctx, tx, actor, workspace)
		if err != nil {
			return err
		}
		if createdProfile {
			if err = database.Audit(ctx, tx, actor, workspace, "apple.profile.created", "apple_service_profile", workspace, correlation, map[string]any{}, map[string]any{"state": profile.State, "revision": profile.Revision}); err != nil {
				return err
			}
		}
		if profile.State != "FOUNDATION_READY" {
			return database.ErrConflict
		}
		if err = entitlements.Reserve(ctx, tx, actor, workspace, plugin.Feature, capability, 1); err != nil {
			return err
		}
		var id string
		err = tx.QueryRow(ctx, `INSERT INTO apple_service_operations(
				workspace_id,kind,state,subject_ref,operation_key,fingerprint,actor_id,expires_at,created_at,updated_at
			) VALUES($1,$2,'REQUESTED',$3,$4,$5,$6,$7,$8,$8) RETURNING id::text`,
			workspace, in.Kind, in.SubjectRef, in.Key, fp, actor, in.ExpiresAt, s.now()).Scan(&id)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO apple_service_operation_events(
				workspace_id,operation_id,sequence,actor_id,from_state,to_state,reason,correlation_id,created_at
			) VALUES($1,$2,1,$3,'','REQUESTED','requested',$4,$5)`,
			workspace, id, actor, correlation, s.now()); err != nil {
			return err
		}
		out, err = s.operation(ctx, tx, workspace, id, false)
		if err != nil {
			return err
		}
		return database.Audit(ctx, tx, actor, workspace, "apple.operation.created", "apple_service_operation", id, correlation, map[string]any{}, map[string]any{"kind": out.Kind, "state": out.State, "revision": out.Revision})
	})
	return out, err
}

func (s *Service) Operations(ctx context.Context, actor, workspace string) ([]Operation, error) {
	out := []Operation{}
	if s.DB == nil {
		return out, database.ErrInvalid
	}
	err := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		if err := entitlements.Require(ctx, tx, actor, workspace, plugin.Feature, plugin.PermissionRead, 0); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id::text,kind,state,subject_ref,revision,expires_at,created_at,updated_at
			FROM apple_service_operations WHERE workspace_id=$1 ORDER BY created_at DESC,id DESC LIMIT 100`, workspace)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item Operation
			if err = rows.Scan(&item.ID, &item.Kind, &item.State, &item.SubjectRef, &item.Revision, &item.ExpiresAt, &item.Created, &item.Updated); err != nil {
				return err
			}
			out = append(out, item)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Service) Transition(ctx context.Context, actor, workspace, operationID, correlation string, in TransitionRequest) (Operation, error) {
	var out Operation
	in.Reason = strings.TrimSpace(in.Reason)
	if s.DB == nil || operationID == "" || !validCorrelation(correlation) || in.Expected < 1 || in.Reason == "" || len(in.Reason) > 500 {
		return out, database.ErrInvalid
	}
	err := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		current, err := s.operation(ctx, tx, workspace, operationID, true)
		if err != nil {
			return err
		}
		capability := Capability(current.Kind)
		if capability == "" {
			return database.ErrInvalid
		}
		if err = entitlements.Require(ctx, tx, actor, workspace, plugin.Feature, capability, 0); err != nil {
			return err
		}
		profile, err := s.profile(ctx, tx, workspace)
		if err != nil {
			return err
		}
		if profile.State != "FOUNDATION_READY" {
			return database.ErrConflict
		}
		if current.Revision != in.Expected {
			return database.ErrConflict
		}
		if current.ExpiresAt != nil && !s.now().Before(*current.ExpiresAt) && in.State != Expired {
			return database.ErrConflict
		}
		if !CanTransition(current.Kind, current.State, in.State) {
			return database.ErrInvalid
		}
		tag, err := tx.Exec(ctx, `UPDATE apple_service_operations
			SET state=$3,revision=revision+1,updated_at=$4
			WHERE workspace_id=$1 AND id::text=$2 AND revision=$5`,
			workspace, operationID, in.State, s.now(), in.Expected)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return database.ErrConflict
		}
		out, err = s.operation(ctx, tx, workspace, operationID, false)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO apple_service_operation_events(
				workspace_id,operation_id,sequence,actor_id,from_state,to_state,reason,correlation_id,created_at
			) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			workspace, operationID, out.Revision, actor, current.State, out.State, in.Reason, correlation, s.now()); err != nil {
			return err
		}
		return database.Audit(ctx, tx, actor, workspace, "apple.operation.transitioned", "apple_service_operation", operationID, correlation, map[string]any{"state": current.State, "revision": current.Revision}, map[string]any{"state": out.State, "revision": out.Revision, "reason": in.Reason})
	})
	return out, err
}

func (s *Service) Events(ctx context.Context, actor, workspace, operationID string) ([]Event, error) {
	out := []Event{}
	if s.DB == nil || operationID == "" {
		return out, database.ErrInvalid
	}
	err := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		if err := entitlements.Require(ctx, tx, actor, workspace, plugin.Feature, plugin.PermissionAudit, 0); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id::text,sequence,from_state,to_state,reason,correlation_id,created_at
			FROM apple_service_operation_events WHERE workspace_id=$1 AND operation_id::text=$2 ORDER BY sequence`, workspace, operationID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item Event
			if err = rows.Scan(&item.ID, &item.Sequence, &item.From, &item.To, &item.Reason, &item.Correlation, &item.Created); err != nil {
				return err
			}
			out = append(out, item)
		}
		return rows.Err()
	})
	return out, err
}
