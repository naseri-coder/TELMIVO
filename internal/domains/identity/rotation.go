package identity

import (
	"context"
	"github.com/jackc/pgx/v5"
	cred "github.com/naseri-coder/TELMIVO/internal/platform/crypto"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
)

// RotateCredential locks one existing installation; repeated rotation is a no-op.
func RotateCredential(ctx context.Context, tx pgx.Tx, actor, workspace, id, reason, correlation string, store cred.RotatingStore) error {
	if reason == "" {
		return database.ErrInvalid
	}
	if !database.Permission(ctx, tx, actor, workspace, "bots.manage") {
		return database.ErrForbidden
	}
	var v cred.EncryptedSecret
	if e := tx.QueryRow(ctx, "SELECT key_id,encrypted_token FROM customer_bots WHERE id=$1 AND workspace_id=$2 FOR UPDATE", id, workspace).Scan(&v.KeyID, &v.Ciphertext); e != nil {
		return e
	}
	n, e := store.Reencrypt(ctx, workspace+":"+id, v)
	if e != nil {
		return e
	}
	if n.KeyID == v.KeyID {
		return nil
	}
	if _, e = tx.Exec(ctx, "UPDATE customer_bots SET key_id=$3,encrypted_token=$4 WHERE id=$1 AND workspace_id=$2", id, workspace, n.KeyID, n.Ciphertext); e != nil {
		return e
	}
	return database.Audit(ctx, tx, actor, workspace, "credential.rotate", "customer_bot", id, correlation, map[string]string{"key_id": v.KeyID}, map[string]string{"key_id": n.KeyID, "reason": reason})
}
