package forms

import (
	"encoding/json"
	"testing"
)

func fixture() Form {
	return Form{Name: "Form", Enabled: true, Access: "PUBLIC_TO_BOT_USER", Fields: []Field{{Key: "choice", Label: "Choice", Type: "SINGLE_CHOICE", Required: true, MaxLength: 100, Options: []string{"yes", "no"}, CRMField: "answer", Tags: map[string]string{"yes": "interested"}}, {Key: "amount", Label: "Amount", Type: "NUMBER", MaxLength: 20, Min: 0, Max: 100}}}
}
func TestTypedValidationAndMappings(t *testing.T) {
	f := fixture()
	fields, tags, ok := f.Validate(map[string]json.RawMessage{"choice": json.RawMessage(`"yes"`), "amount": json.RawMessage(`"10"`)})
	if !ok || fields["answer"] != "yes" || len(tags) != 1 || tags[0] != "interested" {
		t.Fatal(fields, tags, ok)
	}
	for _, raw := range []string{`{"choice":"other"}`, `{"choice":null}`, `{"choice":"yes","unknown":"x"}`, `{"choice":"yes","amount":10}`, `{"choice":"yes","amount":"1.2"}`, `{"choice":"yes","amount":"101"}`, `{"choice":"yes","amount":"01"}`, `{}`} {
		var v map[string]json.RawMessage
		if json.Unmarshal([]byte(raw), &v) != nil {
			t.Fatal(raw)
		}
		if _, _, ok = f.Validate(v); ok {
			t.Fatal("accepted", raw)
		}
	}
}
func TestSchemaBoundsAndProtection(t *testing.T) {
	f := fixture()
	f.Fields[0].Tags["bogus"] = "tag"
	if f.Valid() {
		t.Fatal("mapping outside choices")
	}
	f = fixture()
	f.Access = "ACCESS_POLICY_PROTECTED"
	if f.Valid() {
		t.Fatal("missing policy")
	}
	f = fixture()
	f.Fields = append(f.Fields, f.Fields[0])
	if f.Valid() {
		t.Fatal("duplicate key")
	}
	f = fixture()
	f.Fields[0].Type = "CODE"
	if f.Valid() {
		t.Fatal("executable field")
	}
}
