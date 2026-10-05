package supportplugins

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/businessevents"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/plugins/crm"
)

// ResolveBusinessTx composes CRM in an already admitted, account-locked transaction.
func (s *Service) ResolveBusinessTx(ctx context.Context, tx pgx.Tx, actor string, scope sdk.Scope, in crm.ResolveRequest, fields map[string]string, tags []string) (crm.Contact, error) {
	scope.PluginID = crm.ID
	c, e := s.resolve(ctx, tx, actor, scope, in)
	if e != nil {
		return c, e
	}
	if c.Status != "ACTIVE" {
		return c, database.ErrForbidden
	}
	if len(fields) == 0 && len(tags) == 0 {
		return c, nil
	}
	if c.Fields == nil {
		c.Fields = map[string]string{}
	}
	for k, v := range fields {
		c.Fields[k] = v
	}
	added := false
	for _, v := range tags {
		found := false
		for _, old := range c.Tags {
			found = found || old == v
		}
		if !found {
			c.Tags = append(c.Tags, v)
			added = true
		}
	}
	if !c.Valid() {
		return c, database.ErrInvalid
	}
	_, e = tx.Exec(ctx, "UPDATE crm_contacts SET definition=$4,revision=revision+1 WHERE workspace_id=$1 AND bot_id=$2 AND id=$3", scope.AccountID, scope.BotID, c.ID, body(c))
	if e != nil {
		return c, e
	}
	if e = audit(ctx, tx, actor, scope, "crm.contact.updated", c.ID, map[string]any{"source": "business-plugin"}); e != nil {
		return c, e
	}
	if added {
		e = businessevents.Emit(ctx, tx, actor, scope, "crm.tag.added", c.ID, map[string][]string{"contact.tags": c.Tags})
	}
	return c, e
}
