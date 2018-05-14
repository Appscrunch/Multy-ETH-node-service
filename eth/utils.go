package eth

/*
Copyright 2018 Idealnaya rabota LLC
Licensed under Multy.io license.
See LICENSE for details
*/

import (
	"math/big"
	"time"

	pb "github.com/Appscrunch/Multy-back/node-streamer/eth"
	"github.com/Appscrunch/Multy-back/store"
	"github.com/onrik/ethrpc"
	"gopkg.in/mgo.v2/bson"
)

// TX status constants
const ( // currency id  nsq
	TxStatusAppearedInMempoolIncoming  = 1
	TxStatusAppearedInBlockIncoming    = 2
	TxStatusAppearedInMempoolOutcoming = 3
	TxStatusAppearedInBlockOutcoming   = 4
	TxStatusInBlockConfirmedIncoming   = 5
	TxStatusInBlockConfirmedOutcoming  = 6
	WeiInEthereum                      = 1000000000000000000
)

func newETHtx(hash, from, to string, amount float64, gas, gasprice, nonce int) store.TransactionETH {
	return store.TransactionETH{}
}

// SendRawTransaction sends raw TX
func (client *Client) SendRawTransaction(rawTX string) (string, error) {
	hash, err := client.RPC.EthSendRawTransaction(rawTX)
	if err != nil {
		log.Errorf("SendRawTransaction:rPC.EthSendRawTransaction: %s", err.Error())
		return hash, err
	}
	return hash, err
}

// GetAddressBalance returns address balance
func (client *Client) GetAddressBalance(address string) (big.Int, error) {
	balance, err := client.RPC.EthGetBalance(address, "latest")
	if err != nil {
		log.Errorf("GetAddressBalance:rPC.EthGetBalance: %s", err.Error())
		return balance, err
	}
	return balance, err
}

// GetGasPrice return gas price
func (client *Client) GetGasPrice() (big.Int, error) {
	gas, err := client.RPC.EthGasPrice()
	if err != nil {
		log.Errorf("GetGasPrice:rPC.EthGetBalance: %s", err.Error())
		return gas, err
	}
	return gas, err
}

// GetAddressPendingBalance returns pending balance of selected address
func (client *Client) GetAddressPendingBalance(address string) (big.Int, error) {
	balance, err := client.RPC.EthGetBalance(address, "pending")
	if err != nil {
		log.Errorf("GetAddressPendingBalance:RPC.EthGetBalance: %s", err.Error())
		return balance, err
	}
	log.Errorf("GetAddressPendingBalance %v", balance.String())
	return balance, err
}

// GetAllTxPool return TXs pool
func (client *Client) GetAllTxPool() (map[string]interface{}, error) {
	return client.RPC.TxPoolContent()
}

// GetBlockHeight returns block height
func (client *Client) GetBlockHeight() (int, error) {
	return client.RPC.EthBlockNumber()
}

// GetAddressNonce returns nonce of address
func (client *Client) GetAddressNonce(address string) (int, error) {
	return client.RPC.EthGetTransactionCount(address, "latest")
}

// ResyncAddress resyncs address by TxID
func (client *Client) ResyncAddress(txid string) error {
	tx, err := client.RPC.EthGetTransactionByHash(txid)
	if err != nil {
		return err
	}
	client.parseETHTransaction(*tx, int64(*tx.BlockNumber), true)
	return nil
}

func (client *Client) parseETHTransaction(rawTX ethrpc.Transaction, blockHeight int64, isResync bool) {
	var fromUser store.AddressExtended
	var toUser store.AddressExtended

	client.UserDataM.Lock()
	ud := *client.UsersData
	client.UserDataM.Unlock()

	if udFrom, ok := ud[rawTX.From]; ok {
		fromUser = udFrom
	}

	if udTo, ok := ud[rawTX.To]; ok {
		toUser = udTo
	}

	if fromUser.UserID == toUser.UserID && fromUser.UserID == "" {
		// Not our users tx
		return
	}

	tx := rawToGenerated(rawTX)
	tx.Resync = isResync

	block, err := client.RPC.EthGetBlockByHash(rawTX.BlockHash, false)
	if err != nil {
		if blockHeight == -1 {
			tx.TxpoolTime = time.Now().Unix()
		} else {
			tx.BlockTime = time.Now().Unix()
		}
		tx.BlockHeight = blockHeight
	} else {
		tx.BlockTime = int64(block.Timestamp)
		tx.BlockHeight = int64(block.Number)
	}

	// log.Infof("tx - %v", tx)

	/*
		Fetching tx status and send
	*/
	// from v1 to v1
	if fromUser.UserID == toUser.UserID && fromUser.UserID != "" {
		tx.UserID = fromUser.UserID
		tx.WalletIndex = int32(fromUser.WalletIndex)
		tx.AddressIndex = int32(fromUser.AddressIndex)

		tx.Status = TxStatusAppearedInBlockOutcoming
		if blockHeight == -1 {
			tx.Status = TxStatusAppearedInMempoolOutcoming
		}

		// send to multy-back
		client.TransactionsCh <- tx
	}

	// from v1 to v2 outgoing
	if fromUser.UserID != "" {
		tx.UserID = fromUser.UserID
		tx.WalletIndex = int32(fromUser.WalletIndex)
		tx.AddressIndex = int32(fromUser.AddressIndex)
		tx.Status = TxStatusAppearedInBlockOutcoming
		if blockHeight == -1 {
			tx.Status = TxStatusAppearedInMempoolOutcoming
		}
		// Send to multy-back
		client.TransactionsCh <- tx
	}
	// From v1 to v2 incoming
	if toUser.UserID != "" {
		tx.UserID = toUser.UserID
		tx.WalletIndex = int32(toUser.WalletIndex)
		tx.AddressIndex = int32(toUser.AddressIndex)
		tx.Status = TxStatusAppearedInBlockIncoming
		if blockHeight == -1 {
			tx.Status = TxStatusAppearedInMempoolIncoming
		}
		// Send to multy-back
		client.TransactionsCh <- tx
	}

}

func rawToGenerated(rawTX ethrpc.Transaction) pb.ETHTransaction {
	return pb.ETHTransaction{
		Hash:     rawTX.Hash,
		From:     rawTX.From,
		To:       rawTX.To,
		Amount:   rawTX.Value.String(),
		GasPrice: int64(rawTX.GasPrice.Int64()),
		GasLimit: int64(rawTX.Gas),
		Nonce:    int32(rawTX.Nonce),
	}
}

func isMempoolUpdate(mempool bool, status int) bson.M {
	if mempool {
		return bson.M{
			"$set": bson.M{
				"status": status,
			},
		}
	}
	return bson.M{
		"$set": bson.M{
			"status":    status,
			"blocktime": time.Now().Unix(),
		},
	}
}
