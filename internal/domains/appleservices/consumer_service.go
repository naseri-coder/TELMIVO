package appleservices

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/entitlements"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	plugin "github.com/naseri-coder/TELMIVO/internal/plugins/appleservices"
)

func consumerSubject(actor string) string { return "telmivo-user:" + actor }

func consumerGuidance(goal ConsumerGoal, state State) ConsumerGuidance {
	out := ConsumerGuidance{
		SupportURL:            AppleSupportURL,
		AppleActionOnly:       false,
		SensitiveInputAllowed: false,
		IdentityVerified:      false,
		CompletionMeaning:     "USER_ATTESTED_ONLY",
	}
	switch state {
	case Requested:
		out.Code = "CONFIRM_PRECHECK"
	case Precheck:
		out.Code = "CONTINUE_AT_APPLE"
		out.AppleActionOnly = true
	case AppleActionRequired:
		out.AppleActionOnly = true
		switch goal {
		case ConsumerRecoverAccess:
			out.Code, out.OfficialURL = "RECOVER_ACCESS_AT_APPLE", AppleRecoveryURL
		case ConsumerCheckExisting:
			out.Code, out.OfficialURL = "CHECK_EXISTING_AT_APPLE", AppleAccountURL
		default:
			out.Code, out.OfficialURL = "CREATE_ACCOUNT_AT_APPLE", AppleAccountURL
		}
	case AppleCompleted:
		out.Code = "COMPLETE_APPLE_REQUIRED_VERIFICATION"
		out.AppleActionOnly = true
	case VerificationPending:
		out.Code = "CONFIRM_APPLE_FLOW_COMPLETION"
	case Ready:
		out.Code = "COMPLETE_USER_ATTESTED"
	case Failed:
		out.Code = "FAILED"
	case Cancelled:
		out.Code = "CANCELLED"
	case Expired:
		out.Code = "EXPIRED"
	default:
		out.Code = "UNAVAILABLE"
	}
	return out
}

func consumerTarget(state State, action ConsumerAction) (State, bool) {
	switch {
	case state == Requested && action == ConsumerConfirmPrecheck:
		return Precheck, true
	case state == Precheck && action == ConsumerContinueAtApple:
		return AppleActionRequired, true
	case state == AppleActionRequired && action == ConsumerAppleFlowCompleted:
		return AppleCompleted, true
	case state == AppleCompleted && action == ConsumerVerificationStarted:
		return VerificationPending, true
	case state == VerificationPending && action == ConsumerConfirmCompletion:
		return Ready, true
	case (state == Precheck || state == AppleActionRequired) && action == ConsumerCancel:
		return Cancelled, true
	case (state == Precheck || state == AppleCompleted || state == VerificationPending) && action == ConsumerReportFailure:
		return Failed, true
	default:
		return "", false
	}
}

func (s *Service) consumerTx(ctx context.Context, tx pgx.Tx, workspace, operationID string, lock bool) (ConsumerAssistance, error) {
	var out ConsumerAssistance
	query := `SELECT o.id::text,o.kind,o.state,o.subject_ref,o.revision,o.expires_at,o.created_at,o.updated_at,
		c.goal,c.channel,c.locale,c.region,c.guidance_version,c.completion_source,c.created_at,c.updated_at
		FROM apple_consumer_assistance c
		JOIN apple_service_operations o ON o.id=c.operation_id AND o.workspace_id=c.workspace_id
		WHERE c.workspace_id=$1 AND c.operation_id::text=$2`
	if lock {
		query += " FOR UPDATE OF o,c"
	}
	err := tx.QueryRow(ctx, query, workspace, operationID).Scan(
		&out.Operation.ID, &out.Operation.Kind, &out.Operation.State, &out.Operation.SubjectRef,
		&out.Operation.Revision, &out.Operation.ExpiresAt, &out.Operation.Created, &out.Operation.Updated,
		&out.Goal, &out.Channel, &out.Locale, &out.Region, &out.GuidanceVersion,
		&out.CompletionSource, &out.Created, &out.Updated,
	)
	if err == nil {
		out.Guidance = consumerGuidance(out.Goal, out.Operation.State)
	}
	return out, err
}

func (s *Service) StartConsumer(ctx context.Context, actor, workspace, correlation string, in ConsumerStart) (ConsumerAssistance, error) {
	var out ConsumerAssistance
	in.Locale = strings.TrimSpace(in.Locale)
	in.Region = strings.TrimSpace(in.Region)
	in.Key = strings.TrimSpace(in.Key)
	if s.DB == nil || !validCorrelation(correlation) || !in.Valid(s.now()) {
		return out, database.ErrInvalid
	}
	subject := consumerSubject(actor)
	opInput := CreateRequest{Kind: ConsumerAccount, SubjectRef: subject, Key: in.Key}
	fp := fingerprint(opInput)
	err := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		if err := entitlements.Require(ctx, tx, actor, workspace, plugin.Feature, plugin.ConsumerSetup, 0); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,7500240))", workspace+":"+in.Key); err != nil {
			return err
		}
		replay, saved, err := s.operationByKey(ctx, tx, workspace, in.Key)
		if err == nil {
			if saved != fp || replay.Kind != ConsumerAccount || replay.SubjectRef != subject {
				return database.ErrConflict
			}
			out, err = s.consumerTx(ctx, tx, workspace, replay.ID, false)
			if errors.Is(err, pgx.ErrNoRows) {
				return database.ErrConflict
			}
			if err != nil {
				return err
			}
			if out.Goal != in.Goal || out.Channel != in.Channel || out.Locale != in.Locale || out.Region != in.Region {
				return database.ErrConflict
			}
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
		if err = entitlements.Reserve(ctx, tx, actor, workspace, plugin.Feature, plugin.ConsumerSetup, 1); err != nil {
			return err
		}
		var operationID string
		if err = tx.QueryRow(ctx, `INSERT INTO apple_service_operations(
			workspace_id,kind,state,subject_ref,operation_key,fingerprint,actor_id,expires_at,created_at,updated_at
		) VALUES($1,'CONSUMER_ACCOUNT','REQUESTED',$2,$3,$4,$5,$6,$7,$7) RETURNING id::text`,
			workspace, subject, in.Key, fp, actor, nil, s.now()).Scan(&operationID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO apple_service_operation_events(
			workspace_id,operation_id,sequence,actor_id,from_state,to_state,reason,correlation_id,created_at
		) VALUES($1,$2,1,$3,'','REQUESTED','consumer assistance requested',$4,$5)`,
			workspace, operationID, actor, correlation, s.now()); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO apple_consumer_assistance(
			operation_id,workspace_id,goal,channel,locale,region,created_at,updated_at
		) VALUES($1,$2,$3,$4,$5,$6,$7,$7)`,
			operationID, workspace, in.Goal, in.Channel, in.Locale, in.Region, s.now()); err != nil {
			return err
		}
		out, err = s.consumerTx(ctx, tx, workspace, operationID, false)
		if err != nil {
			return err
		}
		if err = database.Audit(ctx, tx, actor, workspace, "apple.operation.created", "apple_service_operation", operationID, correlation, map[string]any{}, map[string]any{"kind": ConsumerAccount, "state": Requested, "revision": out.Operation.Revision}); err != nil {
			return err
		}
		return database.Audit(ctx, tx, actor, workspace, "apple.consumer.started", "apple_consumer_assistance", operationID, correlation, map[string]any{}, map[string]any{"goal": in.Goal, "channel": in.Channel, "locale": in.Locale, "region": in.Region})
	})
	return out, err
}

func (s *Service) Consumers(ctx context.Context, actor, workspace string) ([]ConsumerAssistance, error) {
	out := []ConsumerAssistance{}
	if s.DB == nil {
		return out, database.ErrInvalid
	}
	err := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		if err := entitlements.Require(ctx, tx, actor, workspace, plugin.Feature, plugin.ConsumerSetup, 0); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT o.id::text,o.kind,o.state,o.subject_ref,o.revision,o.expires_at,o.created_at,o.updated_at,
			c.goal,c.channel,c.locale,c.region,c.guidance_version,c.completion_source,c.created_at,c.updated_at
			FROM apple_consumer_assistance c
			JOIN apple_service_operations o ON o.id=c.operation_id AND o.workspace_id=c.workspace_id
			WHERE c.workspace_id=$1 AND o.subject_ref=$2
			ORDER BY c.created_at DESC,c.operation_id DESC LIMIT 100`, workspace, consumerSubject(actor))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item ConsumerAssistance
			if err = rows.Scan(
				&item.Operation.ID, &item.Operation.Kind, &item.Operation.State, &item.Operation.SubjectRef,
				&item.Operation.Revision, &item.Operation.ExpiresAt, &item.Operation.Created, &item.Operation.Updated,
				&item.Goal, &item.Channel, &item.Locale, &item.Region, &item.GuidanceVersion,
				&item.CompletionSource, &item.Created, &item.Updated,
			); err != nil {
				return err
			}
			item.Guidance = consumerGuidance(item.Goal, item.Operation.State)
			out = append(out, item)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Service) ConsumerEvents(ctx context.Context, actor, workspace, operationID string) ([]Event, error) {
	out := []Event{}
	if s.DB == nil || operationID == "" {
		return out, database.ErrInvalid
	}
	err := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		if err := entitlements.Require(ctx, tx, actor, workspace, plugin.Feature, plugin.ConsumerSetup, 0); err != nil {
			return err
		}
		current, err := s.consumerTx(ctx, tx, workspace, operationID, false)
		if err != nil {
			return err
		}
		if current.Operation.SubjectRef != consumerSubject(actor) {
			return database.ErrForbidden
		}
		rows, err := tx.Query(ctx, `SELECT id::text,sequence,from_state,to_state,reason,correlation_id,created_at
			FROM apple_service_operation_events
			WHERE workspace_id=$1 AND operation_id::text=$2
			ORDER BY sequence`, workspace, operationID)
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

func (s *Service) Consumer(ctx context.Context, actor, workspace, operationID string) (ConsumerAssistance, error) {
	var out ConsumerAssistance
	if s.DB == nil || operationID == "" {
		return out, database.ErrInvalid
	}
	err := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		if err := entitlements.Require(ctx, tx, actor, workspace, plugin.Feature, plugin.ConsumerSetup, 0); err != nil {
			return err
		}
		var err error
		out, err = s.consumerTx(ctx, tx, workspace, operationID, false)
		if err != nil {
			return err
		}
		if out.Operation.SubjectRef != consumerSubject(actor) {
			return database.ErrForbidden
		}
		return nil
	})
	return out, err
}

func (s *Service) ConsumerAction(ctx context.Context, actor, workspace, operationID, correlation string, in ConsumerActionRequest) (ConsumerAssistance, error) {
	var out ConsumerAssistance
	if s.DB == nil || operationID == "" || !validCorrelation(correlation) || !in.Action.Valid() || in.Expected < 1 {
		return out, database.ErrInvalid
	}
	err := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		if err := entitlements.Require(ctx, tx, actor, workspace, plugin.Feature, plugin.ConsumerSetup, 0); err != nil {
			return err
		}
		current, err := s.consumerTx(ctx, tx, workspace, operationID, true)
		if err != nil {
			return err
		}
		if current.Operation.SubjectRef != consumerSubject(actor) {
			return database.ErrForbidden
		}
		if current.Operation.Revision != in.Expected {
			return database.ErrConflict
		}
		profile, err := s.profile(ctx, tx, workspace)
		if err != nil {
			return err
		}
		if profile.State != "FOUNDATION_READY" {
			return database.ErrConflict
		}
		target, ok := consumerTarget(current.Operation.State, in.Action)
		if !ok || !CanTransition(ConsumerAccount, current.Operation.State, target) {
			return database.ErrInvalid
		}
		if current.Operation.ExpiresAt != nil && !s.now().Before(*current.Operation.ExpiresAt) {
			return database.ErrConflict
		}
		tag, err := tx.Exec(ctx, `UPDATE apple_service_operations
			SET state=$3,revision=revision+1,updated_at=$4
			WHERE workspace_id=$1 AND id::text=$2 AND revision=$5`,
			workspace, operationID, target, s.now(), in.Expected)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return database.ErrConflict
		}
		if target == Ready {
			if _, err = tx.Exec(ctx, `UPDATE apple_consumer_assistance
				SET completion_source='USER_ATTESTED',updated_at=$3
				WHERE workspace_id=$1 AND operation_id::text=$2`, workspace, operationID, s.now()); err != nil {
				return err
			}
		} else {
			if _, err = tx.Exec(ctx, `UPDATE apple_consumer_assistance SET updated_at=$3
				WHERE workspace_id=$1 AND operation_id::text=$2`, workspace, operationID, s.now()); err != nil {
				return err
			}
		}
		var next Operation
		next, err = s.operation(ctx, tx, workspace, operationID, false)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO apple_service_operation_events(
			workspace_id,operation_id,sequence,actor_id,from_state,to_state,reason,correlation_id,created_at
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			workspace, operationID, next.Revision, actor, current.Operation.State, target, string(in.Action), correlation, s.now()); err != nil {
			return err
		}
		if err = database.Audit(ctx, tx, actor, workspace, "apple.operation.transitioned", "apple_service_operation", operationID, correlation,
			map[string]any{"state": current.Operation.State, "revision": current.Operation.Revision},
			map[string]any{"state": target, "revision": next.Revision, "consumer_action": in.Action}); err != nil {
			return err
		}
		if err = database.Audit(ctx, tx, actor, workspace, "apple.consumer.action", "apple_consumer_assistance", operationID, correlation,
			map[string]any{"state": current.Operation.State}, map[string]any{"state": target, "action": in.Action}); err != nil {
			return err
		}
		out, err = s.consumerTx(ctx, tx, workspace, operationID, false)
		return err
	})
	return out, err
}
