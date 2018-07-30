package utils

import (
	"github.com/urfave/cli"
)

var (
	// Web3ProviderFlag defines a flag for a mainchain RPC endpoint.
	Web3ProviderFlag = cli.StringFlag{
		Name:  "web3provider",
		Usage: "A mainchain web3 provider string endpoint. Can either be an IPC file string or a WebSocket endpoint. Uses WebSockets by default at ws://127.0.0.1:8546. Cannot be an HTTP endpoint.",
		Value: "ws://127.0.0.1:8546",
	}
	// VrcContractFlag defines a flag for VRC contract address.
	VrcContractFlag = cli.StringFlag{
		Name:  "vrcaddr",
		Usage: "Validator registration contract address. Beacon chain node will listen logs coming from VRC to determine when validator is eligible to participate.",
	}
	// PubKeyFlag defines a flag for validator's public key on the mainchain
	PubKeyFlag = cli.StringFlag{
		Name:  "pubkey",
		Usage: "Validator's public key. Beacon chain node will listen to VRC log to determine when registration has completed based on this public key address.",
	}
)
