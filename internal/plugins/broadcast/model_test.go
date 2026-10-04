package broadcast

import (
	"testing"
)

func TestAudienceAndTransitions(t *testing.T) {
	if !(Audience{Tags: []string{"customer"}}).Valid() || (Audience{Tags: []string{"bad sql?"}}).Valid() {
		t.Fatal("declarative tags")
	}
	for _, v := range [][2]string{{"DRAFT", "START"}, {"RUNNING", "PAUSE"}, {"PAUSED", "RESUME"}, {"RUNNING", "CANCEL"}} {
		if _, ok := Transition(v[0], v[1]); !ok {
			t.Fatal(v)
		}
	}
	for _, v := range [][2]string{{"COMPLETED", "RESUME"}, {"CANCELLED", "START"}, {"RUNNING", "START"}, {"PAUSED", "START"}} {
		if _, ok := Transition(v[0], v[1]); ok {
			t.Fatal(v)
		}
	}
}
