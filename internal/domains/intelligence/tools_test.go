package intelligence

import (
	"encoding/json"
	"fmt"
	provider "github.com/naseri-coder/TELMIVO/internal/platform/ai"
	assistant "github.com/naseri-coder/TELMIVO/internal/plugins/aiassistant"
	"strings"
	"testing"
)

func TestToolAuthorizationIndependentOfModelText(t *testing.T) {
	c := assistant.Config{Tools: []string{"contact.summary", "ticket.note"}}
	for _, tc := range []struct {
		tool provider.Tool
		in   Chat
		ok   bool
	}{{provider.Tool{Name: "contact.summary", Arguments: json.RawMessage(`{}`)}, Chat{}, true}, {provider.Tool{Name: "shell", Arguments: json.RawMessage(`{}`)}, Chat{ConfirmWrites: true}, false}, {provider.Tool{Name: "contact.summary", Arguments: json.RawMessage(`{"url":"http://localhost"}`)}, Chat{}, false}, {provider.Tool{Name: "ticket.note", Arguments: json.RawMessage(`{"value":"note"}`)}, Chat{ConfirmWrites: true}, false}} {
		_, err := validateTool(c, tc.in, tc.tool)
		if (err == nil) != tc.ok {
			t.Fatal(tc, err)
		}
	}
	c.WriteEnabled = true
	if _, e := validateTool(c, Chat{Purpose: "CLASSIFY"}, provider.Tool{Name: "contact.summary", Arguments: json.RawMessage(`{}`)}); e == nil {
		t.Fatal("classification cannot request tools")
	}
	if _, e := validateTool(c, Chat{ConfirmWrites: true}, provider.Tool{Name: "ticket.note", Arguments: json.RawMessage(`{"value":"note"}`)}); e != nil {
		t.Fatal(e)
	}
	if _, e := validateTool(c, Chat{ConfirmWrites: true, User: 10001}, provider.Tool{Name: "ticket.note", Arguments: json.RawMessage(`{"value":"note"}`)}); e == nil {
		t.Fatal("customer write")
	}
}

func TestOperatorKeyLogRepresentation(t *testing.T) {
	in := ModelChange{Key: "SYNTHETIC_SECRET_NEVER_LOGGED"}
	for _, f := range []string{"%v", "%+v", "%#v", "%s"} {
		if strings.Contains(fmt.Sprintf(f, in), in.Key) {
			t.Fatal("operator secret exposed")
		}
	}
}
