package intelligence

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/businessevents"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	workflow "github.com/naseri-coder/TELMIVO/internal/plugins/automation"
)

func scheduleTimezone(w workflow.Workflow) string {
	if w.Schedule.Timezone == "" {
		return "UTC"
	}
	return w.Schedule.Timezone
}

func (s *Service) validateScheduleContext(ctx context.Context, tx pgx.Tx, scope sdk.Scope, w workflow.Workflow) error {
	if w.ScheduleKind() != "CRON" {
		return nil
	}
	if _, err := workflow.NextCron(w.Schedule.Cron, scheduleTimezone(w), s.now()); err != nil {
		return database.ErrInvalid
	}
	explicitContact, explicitUser := w.Schedule.Contact, w.Schedule.User
	event := businessevents.Event{
		Bot:     scope.BotID,
		Ticket:  w.Schedule.Ticket,
		Contact: w.Schedule.Contact,
		User:    w.Schedule.User,
		Fields:  map[string][]string{},
	}
	if event.Ticket != "" || event.Contact != "" {
		if err := s.hydrate(ctx, tx, scope, &event); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return database.ErrInvalid
			}
			return err
		}
	}
	if explicitContact != "" && event.Contact != explicitContact {
		return database.ErrInvalid
	}
	if explicitUser > 0 && event.User > 0 && event.User != explicitUser {
		return database.ErrInvalid
	}
	for _, action := range w.Actions {
		switch {
		case strings.HasPrefix(action.Type, "ticket."), action.Type == "ai.invoke":
			if event.Ticket == "" {
				return database.ErrInvalid
			}
		case strings.HasPrefix(action.Type, "crm."), action.Type == "ai.store_result":
			if event.Contact == "" {
				return database.ErrInvalid
			}
		case action.Type == "telegram.send":
			if event.User <= 0 {
				return database.ErrInvalid
			}
		}
	}
	return nil
}

func (s *Service) syncSchedule(ctx context.Context, tx pgx.Tx, scope sdk.Scope, w workflow.Workflow) error {
	expression, timezone := "", "UTC"
	var next *time.Time
	if w.ScheduleKind() == "CRON" {
		expression = w.Schedule.Cron
		timezone = scheduleTimezone(w)
		if w.Enabled {
			n, err := workflow.NextCron(expression, timezone, s.now())
			if err != nil {
				return database.ErrInvalid
			}
			next = &n
		}
	}
	var currentExpression, currentTimezone string
	var currentNext *time.Time
	err := tx.QueryRow(ctx, "SELECT expression,timezone,next_at FROM automation_schedule_state WHERE workspace_id=$1 AND bot_id=$2 AND workflow_id=$3", scope.AccountID, scope.BotID, w.ID).Scan(&currentExpression, &currentTimezone, &currentNext)
	if err == nil {
		sameActive := next == nil && currentNext == nil || next != nil && currentNext != nil
		if currentExpression == expression && currentTimezone == timezone && sameActive {
			return nil
		}
		_, err = tx.Exec(ctx, "UPDATE automation_schedule_state SET expression=$4,timezone=$5,next_at=$6,last_at=NULL,occurrence=0,revision=revision+1,updated_at=$7 WHERE workspace_id=$1 AND bot_id=$2 AND workflow_id=$3", scope.AccountID, scope.BotID, w.ID, expression, timezone, next, s.now())
		return err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	_, err = tx.Exec(ctx, "INSERT INTO automation_schedule_state(workspace_id,bot_id,workflow_id,expression,timezone,next_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7)", scope.AccountID, scope.BotID, w.ID, expression, timezone, next, s.now())
	return err
}

func (s *Service) materializeSchedules(ctx context.Context, tx pgx.Tx, account string) error {
	// Every edit/materialization path takes the tenant lock before row locks.
	if err := s.lock(ctx, tx, sdk.Scope{AccountID: account}); err != nil {
		return err
	}
	type due struct {
		workflow   string
		bot        string
		next       time.Time
		occurrence int64
	}
	rows, err := tx.Query(ctx, "SELECT workflow_id::text,bot_id::text,next_at,occurrence FROM automation_schedule_state WHERE workspace_id=$1 AND next_at<=$2 ORDER BY next_at,workflow_id FOR UPDATE SKIP LOCKED LIMIT 20", account, s.now())
	if err != nil {
		return err
	}
	items := []due{}
	for rows.Next() {
		var d due
		if err = rows.Scan(&d.workflow, &d.bot, &d.next, &d.occurrence); err != nil {
			rows.Close()
			return err
		}
		items = append(items, d)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	now := s.now()
	for _, d := range items {
		scope := sdk.Scope{AccountID: account, BotID: d.bot, PluginID: workflow.ID}
		w, actor, err := s.workflow(ctx, tx, scope, d.workflow)
		if err != nil {
			return err
		}
		if !w.Enabled || w.ScheduleKind() != "CRON" {
			if _, err = tx.Exec(ctx, "UPDATE automation_schedule_state SET next_at=NULL,revision=revision+1,updated_at=$4 WHERE workspace_id=$1 AND bot_id=$2 AND workflow_id=$3", account, d.bot, d.workflow, now); err != nil {
				return err
			}
			continue
		}
		occurrence := d.occurrence + 1
		eventID := "cron:" + d.next.UTC().Format("20060102T150405Z")
		event := businessevents.Event{
			Bot:     d.bot,
			Actor:   actor,
			Ticket:  w.Schedule.Ticket,
			Contact: w.Schedule.Contact,
			User:    w.Schedule.User,
			Fields: map[string][]string{
				"schedule.kind":       {"CRON"},
				"schedule.fire_at":    {d.next.UTC().Format(time.RFC3339)},
				"schedule.occurrence": {strconv.FormatInt(occurrence, 10)},
			},
			Workflows:   []string{},
			Correlation: eventID,
		}
		if event.Ticket != "" || event.Contact != "" {
			if err = s.hydrate(ctx, tx, scope, &event); err != nil {
				return err
			}
		}
		if _, err = s.enqueue(ctx, tx, scope, d.workflow, eventID, event); err != nil {
			return err
		}
		next, err := workflow.NextCron(w.Schedule.Cron, scheduleTimezone(w), now)
		if err != nil {
			return database.ErrInvalid
		}
		if _, err = tx.Exec(ctx, "UPDATE automation_schedule_state SET last_at=$4,next_at=$5,occurrence=$6,revision=revision+1,updated_at=$7 WHERE workspace_id=$1 AND bot_id=$2 AND workflow_id=$3", account, d.bot, d.workflow, d.next, next, occurrence, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) TickSchedules(ctx context.Context, actor, account string) error {
	return s.DB.Tenant(ctx, actor, account, false, func(tx pgx.Tx) error {
		return s.materializeSchedules(ctx, tx, account)
	})
}
