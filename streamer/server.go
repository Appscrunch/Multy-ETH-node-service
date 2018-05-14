package streamer

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Appscrunch/Multy-ETH-node-service/eth"
	pb "github.com/Appscrunch/Multy-back/node-streamer/eth"
	"github.com/Appscrunch/Multy-back/store"
	"github.com/KristinaEtc/slf"
)

var log = slf.WithContext("streamer")

// Server implements streamer interface and is a gRPC server
type Server struct {
	UsersData *map[string]store.AddressExtended
	M         *sync.Mutex
	ETHCli    *eth.Client
	Info      *store.ServiceInfo
}

// ServiceInfo return service info information
func (s *Server) ServiceInfo(c context.Context, in *pb.Empty) (*pb.ServiceVersion, error) {
	return &pb.ServiceVersion{
		Branch:    s.Info.Branch,
		Commit:    s.Info.Commit,
		Buildtime: s.Info.Buildtime,
		Lasttag:   "",
	}, nil
}

// EventGetGasPrice returns gas price
func (s *Server) EventGetGasPrice(ctx context.Context, in *pb.Empty) (*pb.GasPrice, error) {
	gp, err := s.ETHCli.GetGasPrice()
	if err != nil {
		return &pb.GasPrice{}, err
	}
	return &pb.GasPrice{
		Gas: gp.String(),
	}, nil
}

// EventInitialAdd adds initial user
func (s *Server) EventInitialAdd(c context.Context, ud *pb.UsersData) (*pb.ReplyInfo, error) {
	log.Debugf("EventInitialAdd - %v", ud.Map)

	udMap := map[string]store.AddressExtended{}
	for addr, ex := range ud.GetMap() {
		udMap[strings.ToLower(addr)] = store.AddressExtended{
			UserID:       ex.GetUserID(),
			WalletIndex:  int(ex.GetWalletIndex()),
			AddressIndex: int(ex.GetAddressIndex()),
		}
	}
	s.ETHCli.UserDataM.Lock()
	*s.UsersData = udMap
	s.ETHCli.UserDataM.Unlock()

	log.Debugf("EventInitialAdd - %v", udMap)

	return &pb.ReplyInfo{
		Message: "ok",
	}, nil
}

// EventAddNewAddress us used to add new watch address to existing pairs
func (s *Server) EventAddNewAddress(c context.Context, wa *pb.WatchAddress) (*pb.ReplyInfo, error) {
	s.ETHCli.UserDataM.Lock()
	newMap := *s.UsersData
	s.ETHCli.UserDataM.Unlock()
	if newMap == nil {
		newMap = map[string]store.AddressExtended{}
	}
	_, ok := newMap[wa.Address]
	if ok {
		return &pb.ReplyInfo{
			Message: "err: Address already binded",
		}, nil
	}
	newMap[strings.ToLower(wa.Address)] = store.AddressExtended{
		UserID:       wa.UserID,
		WalletIndex:  int(wa.WalletIndex),
		AddressIndex: int(wa.AddressIndex),
	}

	s.ETHCli.UserDataM.Lock()
	*s.UsersData = newMap
	s.ETHCli.UserDataM.Unlock()

	log.Debugf("EventAddNewAddress - %v", newMap)

	return &pb.ReplyInfo{
		Message: "ok",
	}, nil

}

// EventGetBlockHeight returns block height
func (s *Server) EventGetBlockHeight(ctx context.Context, in *pb.Empty) (*pb.BlockHeight, error) {
	h, err := s.ETHCli.GetBlockHeight()
	if err != nil {
		return &pb.BlockHeight{}, err
	}
	return &pb.BlockHeight{
		Height: int64(h),
	}, nil
}

// EventGetAdressNonce return address nonce
func (s *Server) EventGetAdressNonce(c context.Context, in *pb.AddressToResync) (*pb.Nonce, error) {
	n, err := s.ETHCli.GetAddressNonce(in.GetAddress())
	if err != nil {
		return &pb.Nonce{}, err
	}
	return &pb.Nonce{
		Nonce: int64(n),
	}, nil
}

// EventGetAdressBalance returns address balance
func (s *Server) EventGetAdressBalance(ctx context.Context, in *pb.AddressToResync) (*pb.Balance, error) {
	b, err := s.ETHCli.GetAddressBalance(in.GetAddress())
	if err != nil {
		return &pb.Balance{}, err
	}
	p, err := s.ETHCli.GetAddressPendingBalance(in.GetAddress())
	if err != nil {
		return &pb.Balance{}, err
	}
	return &pb.Balance{
		Balance:        b.String(),
		PendingBalance: p.String(),
	}, nil
}

// EventGetAllMempool returns all the mempool information
func (s *Server) EventGetAllMempool(_ *pb.Empty, stream pb.NodeCommuunications_EventGetAllMempoolServer) error {
	mp, err := s.ETHCli.GetAllTxPool()
	if err != nil {
		return err
	}

	for _, txs := range mp["pending"].(map[string]interface{}) {
		for _, tx := range txs.(map[string]interface{}) {
			gas, err := strconv.ParseInt(tx.(map[string]interface{})["gas"].(string), 0, 64)
			if err != nil {
				log.Errorf("EventGetAllMempool:strconv.ParseInt")
			}
			hash := tx.(map[string]interface{})["hash"].(string)
			stream.Send(&pb.MempoolRecord{
				Category: int32(gas),
				HashTX:   hash,
			})
		}
	}
	return nil
}

type resyncTx struct {
	Message string `json:"message"`
	Result  []struct {
		Hash string `json:"hash"`
	} `json:"result"`
}

// EventResyncAddress recyncs address
func (s *Server) EventResyncAddress(c context.Context, address *pb.AddressToResync) (*pb.ReplyInfo, error) {
	log.Debugf("EventResyncAddress")
	addr := address.GetAddress()
	url := "http://api-rinkeby.etherscan.io/api?sort=asc&endblock=99999999&startblock=0&address=" + addr + "&action=txlist&module=account"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return &pb.ReplyInfo{
			Message: fmt.Sprintf("EventResyncAddress: http.NewRequest = %s", err.Error()),
		}, nil
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return &pb.ReplyInfo{
			Message: fmt.Sprintf("EventResyncAddress: http.DefaultClient.Do = %s", err.Error()),
		}, nil
	}
	defer res.Body.Close()

	reTx := resyncTx{}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return &pb.ReplyInfo{
			Message: fmt.Sprintf("EventResyncAddress: ioutil.ReadAll = %s", err.Error()),
		}, nil
	}

	if err := json.Unmarshal(body, &reTx); err != nil {
		return &pb.ReplyInfo{
			Message: fmt.Sprintf("EventResyncAddress: json.Unmarshal = %s", err.Error()),
		}, nil
	}

	if !strings.Contains(reTx.Message, "OK") {
		return &pb.ReplyInfo{
			Message: fmt.Sprintf("EventResyncAddress: strings.Contains OK  bad resp form 3-party"),
		}, nil
	}

	log.Debugf("EventResyncAddress %d", len(reTx.Result))

	for _, hash := range reTx.Result {
		s.ETHCli.ResyncAddress(hash.Hash)
	}

	return &pb.ReplyInfo{
		Message: "ok",
	}, nil

}

// EventSendRawTx sends raw TXs
func (s *Server) EventSendRawTx(c context.Context, tx *pb.RawTx) (*pb.ReplyInfo, error) {
	hash, err := s.ETHCli.SendRawTransaction(tx.GetTransaction())
	if err != nil {
		return &pb.ReplyInfo{
			Message: "err: wrong raw tx",
		}, fmt.Errorf("err: wrong raw tx %s", err.Error())
	}

	return &pb.ReplyInfo{
		Message: hash,
	}, nil

}

// EventDeleteMempool removes mempool by parameters
func (s *Server) EventDeleteMempool(_ *pb.Empty, stream pb.NodeCommuunications_EventDeleteMempoolServer) error {
	for del := range s.ETHCli.DeleteMempool {
		err := stream.Send(&del)
		if err != nil {
			// HACK:
			log.Errorf("Delete mempool record %s", err.Error())
			i := 0
			for {
				err := stream.Send(&del)
				if err != nil {
					i++
					log.Errorf("Delete mempool record resend attempt(%d) err = %s", i, err.Error())
					time.Sleep(time.Second * 2)
					if i == 3 {
						break
					}
				} else {
					log.Debugf("Delete mempool record resend success on %d attempt", i)
					break
				}
			}
		}
	}
	return nil
}

// EventAddMempoolRecord adds record to mempool
func (s *Server) EventAddMempoolRecord(_ *pb.Empty, stream pb.NodeCommuunications_EventAddMempoolRecordServer) error {
	for add := range s.ETHCli.AddToMempool {
		err := stream.Send(&add)
		if err != nil {
			// HACK:
			log.Errorf("Add mempool record %s", err.Error())
			i := 0
			for {
				err := stream.Send(&add)
				if err != nil {
					i++
					log.Errorf("Add mempool record resend attempt(%d) err = %s", i, err.Error())
					time.Sleep(time.Second * 2)
					if i == 3 {
						break
					}
				} else {
					log.Debugf("Add mempool record resend success on %d attempt", i)
					break
				}
			}
		}
	}
	return nil
}

// NewTx adds new TX
func (s *Server) NewTx(_ *pb.Empty, stream pb.NodeCommuunications_NewTxServer) error {
	for tx := range s.ETHCli.TransactionsCh {
		log.Infof("NewTx history - %v", tx.String())
		err := stream.Send(&tx)
		if err != nil {
			// HACK:
			log.Errorf("NewTx history %s", err.Error())
			i := 0
			for {
				err := stream.Send(&tx)
				if err != nil {
					i++
					log.Errorf("NewTx history resend attempt(%d) err = %s", i, err.Error())
					time.Sleep(time.Second * 2)
					if i == 3 {
						break
					}
				} else {
					log.Debugf("NewTx history resend success on %d attempt", i)
					break
				}
			}

		}
	}
	return nil
}
