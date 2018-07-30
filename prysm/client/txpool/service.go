// Package txpool handles incoming transactions for a sharded Ethereum blockchain.
package txpool

import (
	"github.com/ethereum/go-ethereum/event"
	"github.com/prysmaticlabs/prysm/shared/p2p"
	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("prefix", "txpool")

// TXPool handles a transaction pool for a sharded system.
type TXPool struct {
	p2p              *p2p.Server
	transactionsFeed *event.Feed
}

// NewTXPool creates a new observer instance.
func NewTXPool(p2p *p2p.Server) (*TXPool, error) {
	return &TXPool{p2p: p2p, transactionsFeed: new(event.Feed)}, nil
}

// Start the main routine for a shard transaction pool.
func (p *TXPool) Start() {
	log.Info("Starting shard txpool service")
}

// Stop the main loop for a transaction pool in the shard network.
func (p *TXPool) Stop() error {
	log.Info("Stopping shard txpool service")
	return nil
}
