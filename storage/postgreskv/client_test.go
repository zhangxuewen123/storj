// Copyright (C) 2018 Storj Labs, Inc.
// See LICENSE for copying information.

package postgreskv

import (
	"testing"

	"go.uber.org/zap/zaptest"

	"storj.io/storj/storage/storelogger"
	"storj.io/storj/storage/testsuite"
)

func TestSuite(t *testing.T) {
	store, err := New("postgres://pointerdb:pg-secret-pass@test-postgres-pointerdb/pointerdb?sslmode=disable")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("failed to close db: %v", err)
		}
	}()

	zap := zaptest.NewLogger(t)
	testsuite.RunTests(t, storelogger.New(zap, store))
}

func BenchmarkSuite(b *testing.B) {
	store, err := New("postgres://pointerdb:pg-secret-pass@test-postgres-pointerdb/pointerdb?sslmode=disable")
	if err != nil {
		b.Fatalf("failed to open db: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			b.Fatalf("failed to close db: %v", err)
		}
	}()

	testsuite.RunBenchmarks(b, store)
}
