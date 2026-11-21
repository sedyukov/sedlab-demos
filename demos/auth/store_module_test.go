package main

// The accounts module's Store contract (storetest) against SQLStore on the service's SQLite driver (modernc).

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/sedyukov/sedlab-modules/modules/accounts/go/store"
	"github.com/sedyukov/sedlab-modules/modules/accounts/go/store/storetest"
)

func TestModuleStoreOnModernc(t *testing.T) {
	n := 0
	storetest.Run(t, func() store.Store {
		n++
		dsn := "file:" + filepath.Join(t.TempDir(), fmt.Sprintf("s%d.db", n)) +
			"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_txlock=immediate"
		db, err := sql.Open("sqlite", dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		if err := store.Migrate(context.Background(), db, "sqlite"); err != nil {
			t.Fatal(err)
		}
		return &store.SQLStore{DB: db, Dialect: "sqlite"}
	})
}
