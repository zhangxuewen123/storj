// Copyright (C) 2018 Storj Labs, Inc.
// See LICENSE for copying information.

package postgreskv

import (
	"flag"
	"testing"

	"go.uber.org/zap/zaptest"

	"storj.io/storj/storage/storelogger"
	"storj.io/storj/storage/testsuite"
)

const (
	// this connstring is expected to work under the storj-test docker-compose instance
	defaultPostgresConn = "postgres://pointerdb:pg-secret-pass@test-postgres-pointerdb/pointerdb?sslmode=disable"
)

var (
	testPostgres = flag.String("postgres-test-db", defaultPostgresConn, "postgres test database connection string")
)

func newTestPostgres(t testing.TB) (store *Client, cleanup func()) {
	if *testPostgres == "" {
		t.Skip(`postgres flag missing, example:` + "\n" + defaultPostgresConn)
	}

	pgdb, err := New(*testPostgres)
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	return pgdb, func() {
		if err := pgdb.Close(); err != nil {
			t.Fatalf("failed to close db: %v", err)
		}
	}
}

func TestSuite(t *testing.T) {
	store, cleanup := newTestPostgres(t)
	defer cleanup()

	zap := zaptest.NewLogger(t)
	testsuite.RunTests(t, storelogger.New(zap, store))
}

func BenchmarkSuite(b *testing.B) {
	store, cleanup := newTestPostgres(b)
	defer cleanup()

	testsuite.RunBenchmarks(b, store)
}
