// Copyright (C) 2018 Storj Labs, Inc.
// See LICENSE for copying information.

package satellite

import (
	"go.uber.org/zap"

	"storj.io/storj/pkg/accounting"
	"storj.io/storj/pkg/audit"
	"storj.io/storj/pkg/bwagreement"
	"storj.io/storj/pkg/datarepair/checker"
	"storj.io/storj/pkg/datarepair/irreparable"
	"storj.io/storj/pkg/datarepair/queue"
	"storj.io/storj/pkg/datarepair/repairer"
	"storj.io/storj/pkg/discovery"
	"storj.io/storj/pkg/identity"
	"storj.io/storj/pkg/kademlia"
	"storj.io/storj/pkg/overlay"
	"storj.io/storj/pkg/pointerdb"
	"storj.io/storj/pkg/server"
	"storj.io/storj/pkg/statdb"
	"storj.io/storj/storage"
)

// DB is the master database for the satellite
type DB interface {
	// CreateTables initializes the database
	CreateTables() error
	// Close closes the database
	Close() error

	// BandwidthAgreement returns database for storing bandwidth agreements
	BandwidthAgreement() bwagreement.DB
	// StatDB returns database for storing node statistics
	StatDB() statdb.DB
	// OverlayCache returns database for caching overlay information
	OverlayCache() storage.KeyValueStore
	// Accounting returns database for storing information about data use
	Accounting() accounting.DB
	// RepairQueue returns queue for segments that need repairing
	RepairQueue() queue.RepairQueue
	// Irreparable returns database for failed repairs
	Irreparable() irreparable.DB
}

type FullConfig struct {
	Database string `help:"satellite database connection string" default:"sqlite3://$CONFDIR/master.db"`
	Identity identity.Config
	Config
}

type Config struct {
	Public  server.Config
	Private server.Config

	Kademlia  kademlia.Config
	Overlay   overlay.Config
	Discovery discovery.Config

	PointerDB   pointerdb.Config
	Checker     checker.Config
	Repairer    repairer.Config
	Audit       audit.Config
	BwAgreement bwagreement.Config
}

type Peer struct {
	Log      *zap.Logger
	DB       DB
	Identity *identity.FullIdentity

	Public  *server.Server
	Private *server.Server

	Kademlia  *kademlia.Kademlia
	Overlay   *overlay.Server
	PointerDB *pointerdb.Server
}

func New(log *zap.Logger, identity *identity.FullIdentity, db DB, config *Config) (*Peer, error) {
	peer := &Peer{
		Log:      log,
		DB:       db,
		Identity: identity,
	}
	
	peer.Public = server.New(identity, config.Public)
	peer.Private = server.New(identity, config.Private)

	peer.Kademlia = kademlia.New(peer.Log.Named("kademlia"), db.RoutingTableCache(), pb.NodeType_SATELLITE, ...)
	peer.Overlay = overlay.New(peer.Log.Named("overlay"), db.OverlayCache(), peer.Kademlia)
	
	...

	return peer, nil
}

func (peer *Peer) Register() error {
	peer.Public.Register(peer.Kademlia)
	peer.Public.Register(peer.Overlay)
	peer.Public.Register(peer.PointerDB)

	peer.Private.Register(kademlia.Inspector{peer.Kademlia})
	peer.Private.Register(overlay.Inspector{peer.Overlay})
}

func (peer *Peer) Run(ctx context.Context) error {
	group.Go(peer.Kademlia.Run)
	group.Go(peer.Overlay.Run)
	group.Go(peer.PointerDB.Run)

	group.Go(peer.Public.Run)
	group.Go(peer.Private.Run)

	return group.Wait()
}

func (peer *Peer) Close() error {
	return errs.Combine(
		peer.Public.Close(),
		peer.Private.Close(),
		peer.PointerDB.Close(),
		peer.Overlay.Close(),
		peer.Kademlia.Close(),
	)
}

