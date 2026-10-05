package main

import (
	"context"
	"fmt"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"os"
)

func main() {
	db, e := database.Open(context.Background(), os.Getenv("MIGRATION_DATABASE_URL"), false)
	if e == nil {
		defer db.Pool.Close()
		e = db.Migrate(context.Background())
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, "migration failed:", e)
		os.Exit(1)
	}
}
