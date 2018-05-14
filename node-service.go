package node

/*
Copyright 2018 Idealnaya rabota LLC
Licensed under Multy.io license.
See LICENSE for details
*/

import (
	"fmt"
	"net"
	"sync"

	"github.com/Appscrunch/Multy-ETH-node-service/eth"
	"github.com/Appscrunch/Multy-ETH-node-service/streamer"
	pb "github.com/Appscrunch/Multy-back/node-streamer/eth"
	"github.com/Appscrunch/Multy-back/store"
	"github.com/KristinaEtc/slf"
	"google.golang.org/grpc"
)

var log = slf.WithContext("NodeClient")

// Client is a main struct of service
type Client struct {
	Config     *Configuration
	Instance   *eth.Client
	GRPCserver *streamer.Server
	Clients    *map[string]store.AddressExtended // Address to userid
	// BtcApi     *gobcy.API
}

// Init initializes Multy instance
func Init(conf *Configuration) (*Client, error) {
	cli := &Client{
		Config: conf,
	}

	var usersData = map[string]store.AddressExtended{
		"address": store.AddressExtended{
			UserID:       "kek",
			WalletIndex:  1,
			AddressIndex: 2,
		},
	}

	// Initial initialization of clients data
	cli.Clients = &usersData
	log.Infof("Users data initialization done")

	// Init gRPC server
	lis, err := net.Listen("tcp", conf.GrpcPort)
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %v", err.Error())
	}

	// Creates a new gRPC server
	ETHCli := eth.NewClient(&conf.ETHConf, cli.Clients)
	if err != nil {
		return nil, fmt.Errorf("eth.NewClient initialization: %s", err.Error())
	}
	log.Infof("ETH client initialization done")

	cli.Instance = ETHCli

	s := grpc.NewServer()
	srv := streamer.Server{
		UsersData: cli.Clients,
		M:         &sync.Mutex{},
		ETHCli:    cli.Instance,
		Info:      &conf.ServiceInfo,
	}

	pb.RegisterNodeCommuunicationsServer(s, &srv)
	go s.Serve(lis)

	return cli, nil
}
