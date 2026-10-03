//go:build integration

package integration

import (
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/policies"
	"github.com/naseri-coder/TELMIVO/internal/integrations/media"
	"testing"
	"time"
)

func TestR3CHistoryPagination(t *testing.T) {
	u := fixture(t, true)
	savePolicy(t, u, policies.Version{Key: "r3c-history-" + u.WorkspaceID, Kind: "QUOTA", Feature: "downloads", Workspace: u.WorkspaceID, Enabled: true, EffectiveFrom: time.Now().Add(-time.Second), Reason: "isolated history acceptance", Definition: policies.Definition{Metric: "downloader.successes", Unit: "JOBS", Window: "ROLLING_WINDOW", WindowSeconds: 3600, Timezone: "UTC", Limit: 30, Mode: "USAGE", ReservationSeconds: 900}})
	w := r3cWorker(t, r3cProvider{}, &r3cDelivery{})
	for n := 0; n < 21; n++ {
		id := r3cSubmit(t, u, fmt.Sprintf("r3c-history-%d", n))
		tenant(t, u, func(tx pgx.Tx) error {
			return r3cProduct().Cancel(ctx, tx, u.UserID, u.WorkspaceID, id, "history-cancel")
		})
		must(t, w.RunOne(ctx, u.UserID, u.WorkspaceID, id))
	}
	tenant(t, u, func(tx pgx.Tx) error {
		first, next, e := r3cProduct().History(ctx, tx, u.UserID, u.WorkspaceID, "")
		if e != nil {
			return e
		}
		if len(first) != 20 || next == "" {
			t.Fatal("first page", len(first), next)
		}
		second, end, e := r3cProduct().History(ctx, tx, u.UserID, u.WorkspaceID, next)
		if e != nil {
			return e
		}
		if len(second) != 1 || end != "" {
			t.Fatal("second page", len(second), end)
		}
		for _, j := range first {
			if j.ID == second[0].ID {
				t.Fatal("duplicate page item")
			}
		}
		replay, _, e := r3cProduct().History(ctx, tx, u.UserID, u.WorkspaceID, next)
		if e != nil {
			return e
		}
		if replay[0].ID != second[0].ID {
			t.Fatal("unstable cursor")
		}
		return nil
	})
}

func TestR3CFailurePolicy(t *testing.T) {
	for _, charge := range []bool{false, true} {
		t.Run(fmt.Sprint(charge), func(t *testing.T) {
			u := fixture(t, true)
			savePolicy(t, u, policies.Version{Key: "r3c-failure-policy-" + u.WorkspaceID, Kind: "QUOTA", Feature: "downloads", Workspace: u.WorkspaceID, Enabled: true, EffectiveFrom: time.Now().Add(-time.Second), Reason: "isolated failure settlement", Definition: policies.Definition{Metric: "downloader.successes", Unit: "JOBS", Window: "ROLLING_WINDOW", WindowSeconds: 3600, Timezone: "UTC", Limit: 3, Mode: "USAGE", ReservationSeconds: 900, CountFailure: charge}})
			id := r3cSubmit(t, u, "r3c-failure-policy")
			w := r3cWorker(t, r3cProvider{err: media.Restricted}, &r3cDelivery{})
			must(t, w.RunOne(ctx, u.UserID, u.WorkspaceID, id))
			state := "RELEASED"
			if charge {
				state = "COMMITTED"
			}
			if number(t, u, "SELECT count(*) FROM quota_reservations WHERE workspace_id=$1 AND operation_key='r3c-failure-policy' AND state=$2", u.WorkspaceID, state) != 1 {
				t.Fatal("stored failure policy not honored")
			}
		})
	}
}
