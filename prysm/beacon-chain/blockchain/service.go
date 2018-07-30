package blockchain

import (
	"context"

	"github.com/prysmaticlabs/prysm/beacon-chain/powchain"
	"github.com/prysmaticlabs/prysm/beacon-chain/types"
	"github.com/prysmaticlabs/prysm/shared/database"
	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("prefix", "blockchain")

// ChainService represents a service that handles the internal
// logic of managing the full PoS beacon chain.
type ChainService struct {
	ctx               context.Context
	cancel            context.CancelFunc
	beaconDB          *database.DB
	chain             *BeaconChain
	web3Service       *powchain.Web3Service
	latestBeaconBlock chan *types.Block
	processedHashes   [][32]byte
}

// NewChainService instantiates a new service instance that will
// be registered into a running beacon node.
func NewChainService(ctx context.Context, beaconDB *database.DB, web3Service *powchain.Web3Service) (*ChainService, error) {
	ctx, cancel := context.WithCancel(ctx)
	return &ChainService{ctx, cancel, beaconDB, nil, web3Service, nil, nil}, nil
}

// Start a blockchain service's main event loop.
func (c *ChainService) Start() {
	log.Infof("Starting service")

	beaconChain, err := NewBeaconChain(c.beaconDB.DB())
	if err != nil {
		log.Errorf("Unable to setup blockchain: %v", err)
	}
	c.chain = beaconChain
	go c.updateChainState()
}

// Stop the blockchain service's main event loop and associated goroutines.
func (c *ChainService) Stop() error {
	defer c.cancel()
	log.Info("Stopping service")
	return nil
}

// ProcessedHashes by the chain service.
func (c *ChainService) ProcessedHashes() [][32]byte {
	return c.processedHashes
}

// ProcessBlock accepts a new block for inclusion in the chain.
func (c *ChainService) ProcessBlock(b *types.Block) error {
	c.latestBeaconBlock <- b
	return nil
}

// ContainsBlock checks if a block for the hash exists in the chain.
// This method must be safe to call from a goroutine
func (c *ChainService) ContainsBlock(h [32]byte) bool {
	// TODO
	return false
}

// updateChainState receives a beacon block, computes a new active state and writes it to db. Also
// it checks for if there is an epoch transition. If there is one it computes the validator rewards
// and penalties.
func (c *ChainService) updateChainState() {
	for {
		select {
		case block := <-c.latestBeaconBlock:
			activeStateHash := block.ActiveStateHash()
			log.WithFields(logrus.Fields{"activeStateHash": activeStateHash}).Debug("Received beacon block")

			// TODO: Using latest block hash for seed, this will eventually be replaced by randao
			activeState, err := c.chain.computeNewActiveState(c.web3Service.LatestBlockHash())
			if err != nil {
				log.Errorf("Compute active state failed: %v", err)
			}

			err = c.chain.MutateActiveState(activeState)
			if err != nil {
				log.Errorf("Write active state to disk failed: %v", err)
			}

			currentslot := block.SlotNumber()

			transition := c.chain.isEpochTransition(currentslot)
			if transition {
				if err := c.chain.computeValidatorRewardsAndPenalties(); err != nil {
					log.Errorf("Error computing validator rewards and penalties %v", err)
				}
			}

		case <-c.ctx.Done():
			log.Debug("Chain service context closed, exiting goroutine")
			return
		}
	}
}
