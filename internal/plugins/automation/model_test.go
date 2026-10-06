package automation

import (
	"strings"
	"testing"
)

func validWorkflow() Workflow {
	return Workflow{Name: "bounded", Enabled: true, Trigger: "manual", Conditions: Group{Mode: "ALL"}, Actions: []Action{{Type: "ticket.priority", Value: "HIGH"}}, MaxAttempts: 1}
}
func TestWorkflowBoundaries(t *testing.T) {
	if !validWorkflow().Valid() {
		t.Fatal("valid")
	}
	for _, delay := range []int{0, 86401, MaxDelaySeconds} {
		w := validWorkflow()
		w.DelaySeconds = delay
		if !w.Valid() {
			t.Fatalf("supported bounded delay rejected: %d", delay)
		}
	}
	for _, mutate := range []func(*Workflow){func(w *Workflow) { w.Trigger = "http" }, func(w *Workflow) { w.Actions[0].Type = "shell" }, func(w *Workflow) { w.Actions = make([]Action, MaxActions+1) }, func(w *Workflow) { w.DelaySeconds = MaxDelaySeconds + 1 }, func(w *Workflow) { w.DelaySeconds = -1 }, func(w *Workflow) { w.MaxAttempts = 3 }, func(w *Workflow) { w.Name = strings.Repeat("x", 101) }, func(w *Workflow) { w.Conditions.Mode = "eval" }} {
		w := validWorkflow()
		mutate(&w)
		if w.Valid() {
			t.Fatal("unsafe accepted", w)
		}
	}
}
func TestDeclarativeConditions(t *testing.T) {
	for _, x := range []struct {
		op             string
		actual, expect []string
		want           bool
	}{{"EQUALS", []string{"HIGH"}, []string{"HIGH"}, true}, {"NOT_EQUALS", []string{"LOW"}, []string{"HIGH"}, true}, {"IN", []string{"HIGH"}, []string{"LOW", "HIGH"}, true}, {"NOT_IN", []string{"NORMAL"}, []string{"LOW", "HIGH"}, true}, {"EXISTS", []string{""}, nil, true}, {"NOT_EXISTS", nil, nil, true}, {"CONTAINS", []string{"vip", "lead"}, []string{"vip"}, true}, {"GREATER_THAN", []string{"3"}, []string{"2"}, true}, {"LESS_THAN", []string{"1"}, []string{"2"}, true}, {"GREATER_THAN", []string{"3 garbage"}, []string{"2"}, false}} {
		t.Run(x.op, func(t *testing.T) {
			data := map[string][]string{}
			if x.actual != nil {
				data["ticket.priority"] = x.actual
			}
			g := Group{Mode: "ALL", Items: []Condition{{Field: "ticket.priority", Op: x.op, Values: x.expect}}}
			if g.Match(data) != x.want {
				t.Fatal(g)
			}
		})
	}
	g := Group{Mode: "ALL", Items: []Condition{{Field: "ticket.status", Op: "EXISTS"}, {Field: "ticket.priority", Op: "EXISTS"}}}
	if g.Match(map[string][]string{"ticket.status": {"OPEN"}}) {
		t.Fatal("ALL")
	}
	g.Mode = "ANY"
	if !g.Match(map[string][]string{"ticket.status": {"OPEN"}}) {
		t.Fatal("ANY")
	}
}
