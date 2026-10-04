package crm

import "testing"

func TestContactBounds(t *testing.T) {
	c := Contact{Name: "Customer", Status: "ACTIVE", Tags: []string{"lead"}, Fields: map[string]string{"company": "Example"}}
	if !c.Valid() {
		t.Fatal("valid")
	}
	c.Tags = append(c.Tags, "lead")
	if c.Valid() {
		t.Fatal("duplicate tags")
	}
	c.Tags = nil
	c.Fields["invalid key"] = "x"
	if c.Valid() {
		t.Fatal("field key")
	}
}
