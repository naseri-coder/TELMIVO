package uploads

import "errors"

type Policy struct {
	Enabled          bool
	MaxBytes         int64
	AllowedTypes     map[string]bool
	ApprovalRequired bool
}

func (p Policy) Validate(size int64, kind string) error {
	if !p.Enabled || size <= 0 || p.MaxBytes <= 0 || size > p.MaxBytes || !p.AllowedTypes[kind] {
		return errors.New("upload rejected")
	}
	return nil
}
func Transition(from, to string) error {
	m := map[string][]string{"RECEIVED": {"VALIDATING", "FAILED"}, "VALIDATING": {"PENDING_APPROVAL", "PROCESSING", "REJECTED", "FAILED"}, "PENDING_APPROVAL": {"PROCESSING", "REJECTED"}, "PROCESSING": {"STORED", "FAILED"}}
	for _, v := range m[from] {
		if v == to {
			return nil
		}
	}
	return errors.New("invalid upload transition")
}
