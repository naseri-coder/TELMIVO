// Package ticketing defines the support contract; contact identity belongs to CRM.
package ticketing

import (
	"context"
	"encoding/json"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/plugins/crm"
	files "github.com/naseri-coder/TELMIVO/internal/plugins/filemanager"
	"strings"
	"time"
)

const ID = "ticketing"

var transitions = map[string][]string{"OPEN": {"PENDING_CUSTOMER", "PENDING_AGENT", "RESOLVED", "CLOSED"}, "PENDING_CUSTOMER": {"OPEN", "PENDING_AGENT", "RESOLVED", "CLOSED"}, "PENDING_AGENT": {"OPEN", "PENDING_CUSTOMER", "RESOLVED", "CLOSED"}, "RESOLVED": {"CLOSED"}, "CLOSED": {}}

func Transition(from, to string, reopen bool) bool {
	if reopen {
		return (from == "RESOLVED" || from == "CLOSED") && to == "OPEN"
	}
	for _, x := range transitions[from] {
		if x == to {
			return true
		}
	}
	return false
}
func Priority(s string) bool { return s == "LOW" || s == "NORMAL" || s == "HIGH" || s == "URGENT" }
func Active(s string) bool   { return s == "OPEN" || s == "PENDING_CUSTOMER" || s == "PENDING_AGENT" }

type Attachment struct {
	FileID   string `json:"file_id"`
	UniqueID string `json:"file_unique_id"`
	Kind     string `json:"type"`
	Title    string `json:"title"`
}

func AttachmentsValid(a []Attachment) bool {
	if len(a) > 4 {
		return false
	}
	for _, x := range a {
		if !(files.File{FileID: x.FileID, UniqueID: x.UniqueID, Kind: x.Kind, Title: x.Title}).Valid() {
			return false
		}
	}
	return true
}

type Department struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	Enabled           bool   `json:"enabled"`
	Order             int    `json:"ordering"`
	ResponseSeconds   int64  `json:"response_seconds"`
	ResolutionSeconds int64  `json:"resolution_seconds"`
	Revision          int64  `json:"revision"`
}

func (d Department) Valid() bool {
	return strings.TrimSpace(d.Name) != "" && len(d.Name) <= 100 && len(d.Description) <= 500 && d.Order >= 0 && d.ResponseSeconds >= 0 && d.ResolutionSeconds >= 0 && d.ResponseSeconds <= 31536000 && d.ResolutionSeconds <= 31536000
}

type Ticket struct {
	Reference          string     `json:"reference"`
	ContactID          string     `json:"contact_id"`
	ContactName        string     `json:"contact_name"`
	DepartmentID       string     `json:"department_id"`
	DepartmentName     string     `json:"department_name"`
	Subject            string     `json:"subject"`
	Status             string     `json:"status"`
	Priority           string     `json:"priority"`
	Assignee           string     `json:"assignee"`
	Queue              string     `json:"queue"`
	Tags               []string   `json:"tags"`
	Revision           int64      `json:"revision"`
	Created            time.Time  `json:"created_at"`
	Updated            time.Time  `json:"updated_at"`
	Closed             *time.Time `json:"closed_at"`
	FirstResponse      *time.Time `json:"first_response_at"`
	Resolved           *time.Time `json:"resolved_at"`
	ResponseDue        *time.Time `json:"response_due_at"`
	ResolutionDue      *time.Time `json:"resolution_due_at"`
	ResponseBreached   bool       `json:"response_breached"`
	ResolutionBreached bool       `json:"resolution_breached"`
}

func (t *Ticket) Evaluate(now time.Time) {
	t.Queue = "UNASSIGNED"
	if t.Assignee != "" {
		t.Queue = "ASSIGNED"
	}
	if t.Status == "PENDING_CUSTOMER" {
		t.Queue = "WAITING"
	}
	t.ResponseBreached = t.ResponseDue != nil && (t.FirstResponse == nil && !now.Before(*t.ResponseDue) || t.FirstResponse != nil && t.FirstResponse.After(*t.ResponseDue))
	t.ResolutionBreached = t.ResolutionDue != nil && (t.Resolved == nil && !now.Before(*t.ResolutionDue) || t.Resolved != nil && t.Resolved.After(*t.ResolutionDue))
}

type Message struct {
	ID          string       `json:"id"`
	Kind        string       `json:"kind"`
	Body        string       `json:"body"`
	Attachments []Attachment `json:"attachments"`
	Actor       string       `json:"actor"`
	Delivery    string       `json:"delivery"`
	Created     time.Time    `json:"created_at"`
}
type Template struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Body     string `json:"body"`
	Enabled  bool   `json:"enabled"`
	Revision int64  `json:"revision"`
}
type Inbound struct {
	User        int64        `json:"user"`
	Name        string       `json:"name"`
	Username    string       `json:"username"`
	Locale      string       `json:"locale"`
	Department  string       `json:"department"`
	Body        string       `json:"body"`
	Key         string       `json:"key"`
	Attachments []Attachment `json:"attachments"`
}
type Contacts interface {
	Resolve(context.Context, crm.ResolveRequest) (crm.Contact, error)
}
type Plugin struct{ Runtime sdk.Invoker }

func (p Plugin) Receive(ctx context.Context, in Inbound) (Ticket, error) {
	b, _ := json.Marshal(in)
	out, e := p.Runtime.Invoke(ctx, "tickets.write", b)
	var t Ticket
	if e == nil {
		e = json.Unmarshal(out, &t)
	}
	return t, e
}
func Manifest() sdk.Manifest {
	return sdk.Manifest{SchemaVersion: 1, PluginID: ID, Name: "پشتیبانی و تیکت", Description: "گفت‌وگوی پشتیبانی، صف و کارشناس", Version: "1.0.0", CoreVersions: sdk.VersionRange{Minimum: "0.3.0", MaximumExclusive: "1.0.0"}, DataVersion: 1, Permissions: []string{"tickets.read", "tickets.write", "telegram.send"}, Dependencies: []sdk.Dependency{{PluginID: crm.ID, Versions: sdk.VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"}}}, Settings: sdk.SettingsSchema{Properties: map[string]sdk.Setting{}}, Extensions: sdk.Extensions{PricingPolicy: ID, QuotaPolicy: "ticketing.messages.monthly"}}
}
