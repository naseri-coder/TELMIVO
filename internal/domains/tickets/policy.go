package tickets

import "errors"

var transitions = map[string][]string{
	"OPEN":                 {"PENDING", "WAITING_FOR_CUSTOMER", "WAITING_FOR_AGENT", "IN_PROGRESS", "RESOLVED", "CLOSED"},
	"PENDING":              {"IN_PROGRESS", "WAITING_FOR_CUSTOMER", "WAITING_FOR_AGENT", "RESOLVED", "CLOSED"},
	"WAITING_FOR_CUSTOMER": {"IN_PROGRESS", "WAITING_FOR_AGENT", "RESOLVED", "CLOSED"},
	"WAITING_FOR_AGENT":    {"IN_PROGRESS", "WAITING_FOR_CUSTOMER", "RESOLVED", "CLOSED"},
	"IN_PROGRESS":          {"PENDING", "WAITING_FOR_CUSTOMER", "WAITING_FOR_AGENT", "RESOLVED", "CLOSED"},
	"RESOLVED":             {"OPEN", "CLOSED"}, "CLOSED": {"OPEN"},
}

func Transition(from, to string) error {
	for _, v := range transitions[from] {
		if v == to {
			return nil
		}
	}
	return errors.New("invalid ticket transition")
}
func ValidPriority(p string) bool { return p == "LOW" || p == "NORMAL" || p == "HIGH" || p == "URGENT" }
