package eth

/*
Copyright 2017 Idealnaya rabota LLC
Licensed under Multy.io license.
See LICENSE for details
*/

import (
	"context"
	"sync"

	pb "github.com/Appscrunch/Multy-back/node-streamer/eth"
	"github.com/Appscrunch/Multy-back/store"
	"github.com/KristinaEtc/slf"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/onrik/ethrpc"
)

var log = slf.WithContext("eth")

// Client contains all the data
type Client struct {
	RPC            *ethrpc.EthRPC
	Client         *rpc.Client
	config         *Conf
	TransactionsCh chan pb.ETHTransaction
	DeleteMempool  chan pb.MempoolToDelete
	AddToMempool   chan pb.MempoolRecord
	UsersData      *map[string]store.AddressExtended
	UserDataM      *sync.Mutex
}

// Conf is a config struct
type Conf struct {
	Address string
	RPCPort string
	WsPort  string
}

// NewClient adds new client
func NewClient(conf *Conf, usersData *map[string]store.AddressExtended) *Client {
	c := &Client{
		config:         conf,
		TransactionsCh: make(chan pb.ETHTransaction),
		DeleteMempool:  make(chan pb.MempoolToDelete),
		AddToMempool:   make(chan pb.MempoolRecord),
		UsersData:      usersData,
		UserDataM:      &sync.Mutex{},
	}
	go c.RunProcess()
	return c
}

// RunProcess interacts with rpc
func (c *Client) RunProcess() error {

	log.Info("Run ETH Process")
	// c.Rpc = ethrpc.NewEthRPC("http://" + c.config.Address + c.config.RPCPort)
	c.RPC = ethrpc.NewEthRPC("http://" + c.config.Address + c.config.RPCPort)
	log.Infof("ETH RPC Connection %s", "http://"+c.config.Address+c.config.RPCPort)

	_, err := c.RPC.EthNewPendingTransactionFilter()
	if err != nil {
		log.Errorf("NewClient:EthNewPendingTransactionFilter: %s", err.Error())
		return err
	}

	// client, err := rpc.Dial("ws://" + c.config.Address + c.config.WsPort)
	client, err := rpc.Dial("ws://" + c.config.Address + c.config.WsPort)
	defer client.Close()
	if err != nil {
		log.Errorf("Dial err: %s", err.Error())
		return err
	}
	c.Client = client
	log.Infof("ETH RPC Connection %s", "ws://"+c.config.Address+c.config.WsPort)

	ch := make(chan interface{})

	_, err = c.Client.Subscribe(context.Background(), "eth", ch, "newHeads")
	if err != nil {
		log.Errorf("Run: client.Subscribe: newHeads %s", err.Error())
		return err
	}

	_, err = c.Client.Subscribe(context.Background(), "eth", ch, "newPendingTransactions")
	if err != nil {
		log.Errorf("Run: client.Subscribe: newPendingTransactions %s", err.Error())
		return err
	}

	for {
		switch v := (<-ch).(type) {
		default:
			log.Errorf("Not found type: %v", v)
		case string:
			// tx pool transaction
			go c.txpoolTransaction(v)
			// fmt.Println(v)
		case map[string]interface{}:
			// tx block transactions
			// fmt.Println(v)
			go c.blockTransaction(v["hash"].(string))
		}
	}
	// TODO: Fix unreachable code
	return nil
}
