package automation

import (
	"testing"
	"time"
)

func TestCronValidationAndNext(t *testing.T) {
	if !ValidCron("*/15 9-17 * * 1-5", "UTC") {
		t.Fatal("valid cron rejected")
	}
	if ValidCron("* * *", "UTC") || ValidCron("* * * * *", "No/Such_Zone") || ValidCron("61 * * * *", "UTC") {
		t.Fatal("invalid cron accepted")
	}
	start := time.Date(2026, time.October, 5, 8, 59, 40, 0, time.UTC)
	next, err := NextCron("*/15 9-17 * * 1-5", "UTC", start)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, time.October, 5, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next=%s want=%s", next, want)
	}
}

func TestCronDayAndWeekdaySemantics(t *testing.T) {
	start := time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC)
	next, err := NextCron("0 12 10 * 2", "UTC", start)
	if err != nil {
		t.Fatal(err)
	}
	if next.Day() != 6 || next.Weekday() != time.Tuesday {
		t.Fatalf("expected day-of-month/day-of-week OR semantics, got %s", next)
	}
}

func TestWorkflowScheduleValidation(t *testing.T) {
	base := Workflow{Name: "x", Enabled: true, Trigger: "manual", Conditions: Group{Mode: "ALL"}, Actions: []Action{{Type: "telegram.send", Value: "x"}}, MaxAttempts: 1}
	if !base.Valid() {
		t.Fatal("legacy event workflow invalid")
	}
	cron := base
	cron.Trigger = "schedule.cron"
	cron.Schedule = Schedule{Kind: "CRON", Cron: "0 * * * *", Timezone: "UTC", User: 10001}
	if !cron.Valid() {
		t.Fatal("cron workflow invalid")
	}
	sla := base
	sla.Trigger = "ticket.created"
	sla.Schedule = Schedule{Kind: "SLA", SLASeconds: 300}
	if !sla.Valid() || sla.Delay() != 5*time.Minute {
		t.Fatal("sla workflow invalid")
	}
	bad := cron
	bad.DelaySeconds = 1
	if bad.Valid() {
		t.Fatal("cron accepted additional event delay")
	}
}
