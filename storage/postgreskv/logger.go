// Copyright (C) 2018 Storj Labs, Inc.
// See LICENSE for copying information.

package postgreskv

import (
	"go.uber.org/zap"

	"storj.io/storj/storage"
	"storj.io/storj/storage/storelogger"
)

// NewClient instantiates a new PostgreSQL key/value-storage client given postgresql-compatible db URL
func NewClient(log *zap.Logger, dbURL string) (storage.KeyValueStore, error) {
	client, err := New(dbURL)
	db := storelogger.New(log, client)
	return db, err
}
