package ticketing

import (
	"testing"
	"time"
)

func TestLifecycleAndSLA(t *testing.T) {
	if !Transition("OPEN", "PENDING_AGENT", false) || Transition("CLOSED", "OPEN", false) || !Transition("CLOSED", "OPEN", true) || Transition("OPEN", "OPEN", true) {
		t.Fatal("transition")
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	due := now.Add(time.Hour)
	x := Ticket{ResponseDue: &due, ResolutionDue: &due, Status: "OPEN"}
	x.Evaluate(now)
	if x.ResponseBreached || x.Queue != "UNASSIGNED" {
		t.Fatal(x)
	}
	x.Evaluate(due)
	if !x.ResponseBreached || !x.ResolutionBreached {
		t.Fatal(x)
	}
	if AttachmentsValid([]Attachment{{FileID: "bad url://", Kind: "document"}}) {
		t.Fatal("attachment")
	}
}
