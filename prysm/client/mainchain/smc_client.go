// Package mainchain defines services that interacts with a Geth node via RPC.
// This package is useful for an actor in a sharded system to interact with
// a Sharding Manager Contract.
package mainchain

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	gethTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/prysmaticlabs/prysm/client/contracts"
	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("prefix", "mainchain")

// ClientIdentifier tells us what client the node we interact with over RPC is running.
const ClientIdentifier = "geth"

// SMCClient defines a struct that interacts with a
// mainchain node via RPC. Specifically, it aids in SMC bindings that are useful
// to other sharding services.
type SMCClient struct {
	endpoint     string             // Endpoint to JSON RPC.
	dataDirPath  string             // Path to the data directory.
	depositFlag  bool               // Keeps track of the deposit option passed in as via CLI flags.
	passwordFile string             // Path to the account password file.
	client       *ethclient.Client  // Ethereum RPC client.
	keystore     *keystore.KeyStore // Keystore containing the single signer.
	smc          *contracts.SMC     // The deployed sharding management contract.
	rpcClient    *rpc.Client        // The RPC client connection to the main geth node.
}

// NewSMCClient constructs a new instance of an SMCClient.
func NewSMCClient(endpoint string, dataDirPath string, depositFlag bool, passwordFile string) (*SMCClient, error) {
	config := &node.Config{
		DataDir: dataDirPath,
	}

	scryptN, scryptP, keydir, err := config.AccountConfig()
	if err != nil {
		return nil, err
	}

	ks := keystore.NewKeyStore(keydir, scryptN, scryptP)

	smcClient := &SMCClient{
		keystore:     ks,
		endpoint:     endpoint,
		depositFlag:  depositFlag,
		dataDirPath:  dataDirPath,
		passwordFile: passwordFile,
	}

	return smcClient, nil
}

// Start the SMC Client and connect to running geth node.
func (s *SMCClient) Start() {
	// Sets up a connection to a Geth node via RPC.
	rpcClient, err := dialRPC(s.endpoint)
	if err != nil {
		log.Panicf("Cannot start rpc client: %v", err)
		return
	}

	s.rpcClient = rpcClient
	s.client = ethclient.NewClient(rpcClient)

	// Check account existence and unlock account before starting.
	accounts := s.keystore.Accounts()
	if len(accounts) == 0 {
		log.Panic("No accounts found")
		return
	}

	if err := s.unlockAccount(accounts[0]); err != nil {
		log.Panicf("Cannot unlock account: %v", err)
		return
	}

	// Initializes bindings to SMC.
	smc, err := initSMC(s)
	if err != nil {
		log.Panicf("Failed to initialize SMC: %v", err)
		return
	}

	s.smc = smc
}

// Stop SMCClient immediately. This cancels any pending RPC connections.
func (s *SMCClient) Stop() error {
	s.rpcClient.Close()
	return nil
}

// CreateTXOpts creates a *TransactOpts with a signer using the default account on the keystore.
func (s *SMCClient) CreateTXOpts(value *big.Int) (*bind.TransactOpts, error) {
	account := s.Account()

	return &bind.TransactOpts{
		From:  account.Address,
		Value: value,
		Signer: func(signer gethTypes.Signer, addr common.Address, tx *gethTypes.Transaction) (*gethTypes.Transaction, error) {
			networkID, err := s.client.NetworkID(context.Background())
			if err != nil {
				return nil, fmt.Errorf("unable to fetch networkID: %v", err)
			}
			return s.keystore.SignTx(*account, tx, networkID /* chainID */)
		},
	}, nil
}

// Account to use for sharding transactions.
func (s *SMCClient) Account() *accounts.Account {
	accounts := s.keystore.Accounts()
	return &accounts[0]
}

// ChainReader for interacting with the chain.
func (s *SMCClient) ChainReader() ethereum.ChainReader {
	return ethereum.ChainReader(s.client)
}

// BlockByNumber helper function for fetching a mainchain block by its block
// number.
func (s *SMCClient) BlockByNumber(ctx context.Context, number *big.Int) (*gethTypes.Block, error) {
	return s.ChainReader().BlockByNumber(ctx, number)
}

// SubscribeNewHead helper function for subscribing to new mainchain headers.
func (s *SMCClient) SubscribeNewHead(ctx context.Context, ch chan<- *gethTypes.Header) (ethereum.Subscription, error) {
	return s.ChainReader().SubscribeNewHead(ctx, ch)
}

// SMCCaller to interact with the sharding manager contract.
func (s *SMCClient) SMCCaller() *contracts.SMCCaller {
	if s.smc == nil {
		return nil
	}
	return &s.smc.SMCCaller
}

// SMCTransactor allows us to send tx's to the SMC programmatically.
func (s *SMCClient) SMCTransactor() *contracts.SMCTransactor {
	if s.smc == nil {
		return nil
	}
	return &s.smc.SMCTransactor
}

// SMCFilterer allows for easy filtering of events from the Sharding Manager Contract.
func (s *SMCClient) SMCFilterer() *contracts.SMCFilterer {
	if s.smc == nil {
		return nil
	}
	return &s.smc.SMCFilterer
}

// WaitForTransaction waits for transaction to be mined and returns an error if it takes
// too long.
func (s *SMCClient) WaitForTransaction(ctx context.Context, hash common.Hash, durationInSeconds time.Duration) error {

	ctxTimeout, cancel := context.WithTimeout(ctx, durationInSeconds*time.Second)

	for pending, err := true, error(nil); pending; _, pending, err = s.client.TransactionByHash(ctxTimeout, hash) {
		if err != nil {
			cancel()
			return fmt.Errorf("unable to retrieve transaction: %v", err)
		}
		if ctxTimeout.Err() != nil {
			cancel()
			return fmt.Errorf("transaction timed out, transaction was not able to be mined in the duration: %v", ctxTimeout.Err())
		}
	}
	cancel()
	ctxTimeout.Done()
	log.Infof("Transaction: %s has been mined", hash.Hex())
	return nil
}

// TransactionReceipt allows an SMCClient to retrieve transaction receipts on
// the mainchain by hash.
func (s *SMCClient) TransactionReceipt(hash common.Hash) (*gethTypes.Receipt, error) {

	receipt, err := s.client.TransactionReceipt(context.Background(), hash)
	if err != nil {
		return nil, err
	}

	return receipt, err
}

// DepositFlag returns true for cli flag --deposit.
func (s *SMCClient) DepositFlag() bool {
	return s.depositFlag
}

// SetDepositFlag updates the deposit flag property of SMCClient.
func (s *SMCClient) SetDepositFlag(deposit bool) {
	s.depositFlag = deposit
}

// DataDirPath returns the datadir flag as a string.
func (s *SMCClient) DataDirPath() string {
	return s.dataDirPath
}

// unlockAccount will unlock the specified account using utils.PasswordFileFlag
// or empty string if unset.
func (s *SMCClient) unlockAccount(account accounts.Account) error {
	pass := ""

	if s.passwordFile != "" {
		file, err := os.Open(s.passwordFile)
		if err != nil {
			return fmt.Errorf("unable to open file containing account password %s. %v", s.passwordFile, err)
		}
		scanner := bufio.NewScanner(file)
		scanner.Split(bufio.ScanWords)
		if !scanner.Scan() {
			err = scanner.Err()
			if err != nil {
				return fmt.Errorf("unable to read contents of file %v", err)
			}
			return errors.New("password not found in file")
		}

		pass = scanner.Text()
	}

	return s.keystore.Unlock(account, pass)
}

// Sign signs the hash of collationHeader contents by
// using default account on keystore and returns signed signature.
func (s *SMCClient) Sign(hash common.Hash) ([]byte, error) {
	account := s.Account()
	return s.keystore.SignHash(*account, hash.Bytes())
}

// GetShardCount gets the count of the total shards
// currently operating in the sharded universe.
func (s *SMCClient) GetShardCount() (int64, error) {
	shardCount, err := s.SMCCaller().ShardCount(&bind.CallOpts{})
	if err != nil {
		return 0, err
	}
	return shardCount.Int64(), nil
}
