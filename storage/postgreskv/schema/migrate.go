// Copyright (C) 2018 Storj Labs, Inc.
// See LICENSE for copying information.

//go:generate go-bindata -o data.go -pkg schema -ignore ".*go" .

package schema

import (
	"database/sql"

	_ "github.com/go-bindata/go-bindata"
	"github.com/golang-migrate/migrate/v3"
	"github.com/golang-migrate/migrate/v3/database/postgres"
	"github.com/golang-migrate/migrate/v3/source/go_bindata"
)

func PrepareDB(db *sql.DB) error {
	srcDriver, err := bindata.WithInstance(bindata.Resource(AssetNames(),
		func(name string) ([]byte, error) {
			return Asset(name)
		}))
	if err != nil {
		return err
	}
	dbDriver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return err
	}
	m, err := migrate.NewWithInstance("go-bindata migrations", srcDriver, "postgreskv db", dbDriver)
	if err != nil {
		return err
	}
	err = m.Up()
	if err == migrate.ErrNoChange {
		err = nil
	}
	return err
}
