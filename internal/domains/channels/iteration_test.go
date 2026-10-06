package channels

import (
	"errors"
	"github.com/jackc/pgx/v5"
	"testing"
)

type interruptedChannelRows struct {
	pgx.Rows
	advanced, closed bool
	failure          error
}

func (r *interruptedChannelRows) Next() bool {
	if r.advanced {
		return false
	}
	r.advanced = true
	return true
}
func (r *interruptedChannelRows) Scan(dest ...any) error {
	*dest[0].(*string) = "synthetic-channel"
	return nil
}
func (r *interruptedChannelRows) Err() error { return r.failure }
func (r *interruptedChannelRows) Close()     { r.closed = true }
func TestChannelListRejectsPartialResultStream(t *testing.T) {
	failure := errors.New("synthetic interrupted channel stream")
	rows := &interruptedChannelRows{failure: failure}
	ids, err := readChannelIDs(rows)
	if !errors.Is(err, failure) || len(ids) != 0 || !rows.closed {
		t.Fatalf("partial result accepted: %v %v closed=%v", ids, err, rows.closed)
	}
}
