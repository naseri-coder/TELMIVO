package intelligence

import (
	"errors"
	"github.com/jackc/pgx/v5"
	"testing"
)

type interruptedMemberRows struct {
	pgx.Rows
	advanced, closed bool
	failure          error
}

func (r *interruptedMemberRows) Next() bool {
	if r.advanced {
		return false
	}
	r.advanced = true
	return true
}
func (r *interruptedMemberRows) Scan(dest ...any) error {
	*dest[0].(*string) = "synthetic-actor"
	*dest[1].(*string) = "synthetic-workspace"
	return nil
}
func (r *interruptedMemberRows) Err() error { return r.failure }
func (r *interruptedMemberRows) Close()     { r.closed = true }
func TestWorkerRejectsPartialOwnerBatch(t *testing.T) {
	failure := errors.New("synthetic interrupted result stream")
	rows := &interruptedMemberRows{failure: failure}
	members, err := readAutomationMembers(rows)
	if !errors.Is(err, failure) || len(members) != 0 || !rows.closed {
		t.Fatalf("partial batch accepted: %v %v closed=%v", members, err, rows.closed)
	}
}
