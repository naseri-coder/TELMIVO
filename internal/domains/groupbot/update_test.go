package groupbot

import (
	"encoding/json"
	"testing"
)

func TestClaimableMembership(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{"member is acknowledged but not claimed", `{"update_id":1,"my_chat_member":{"chat":{"id":-1001,"type":"supergroup","title":"x"},"from":{"id":10},"new_chat_member":{"status":"member"}}}`, false},
		{"administrator triggers claim", `{"update_id":2,"my_chat_member":{"chat":{"id":-1001,"type":"supergroup","title":"x"},"from":{"id":10},"new_chat_member":{"status":"administrator"}}}`, true},
		{"left is acknowledged but not claimed", `{"update_id":3,"my_chat_member":{"chat":{"id":-1001,"type":"supergroup","title":"x"},"from":{"id":10},"new_chat_member":{"status":"left"}}}`, false},
		{"message cannot claim", `{"update_id":4,"message":{"message_id":1,"chat":{"id":-1001,"type":"supergroup","title":"x"},"from":{"id":10},"text":"/status"}}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var u Update
			if err := json.Unmarshal([]byte(tc.raw), &u); err != nil {
				t.Fatal(err)
			}
			if got := claimableMembership(u); got != tc.want {
				t.Fatalf("claimableMembership=%v want %v", got, tc.want)
			}
		})
	}
}
