package main

import (
	"crypto/tls"
	"fmt"
	"ongrid-thrift/ongrid2"

	"git.apache.org/thrift.git/lib/go/thrift"
)

func runServer(transportFactory thrift.TTransportFactory, protocolFactory thrift.TProtocolFactory, addr string, secure bool) error {
	var transport thrift.TServerTransport
	var err error
	if secure {
		cfg := new(tls.Config)
		if cert, err := tls.LoadX509KeyPair("keys/server.crt", "keys/server.key"); err == nil {
			cfg.Certificates = append(cfg.Certificates, cert)
		} else {
			return err
		}
		transport, err = thrift.NewTSSLServerSocket(addr, cfg)
	} else {
		transport, err = thrift.NewTServerSocket(addr)
	}

	if err != nil {
		return err
	}

	hDB := NewDBHandler()
	hOngrid := NewOngridHandler()
	//processor := ongrid2.NewIntergridProcessor(handler)
	dbProcessor := ongrid2.NewDBProcessor(hDB)
	ongridProcessor := ongrid2.NewOngridProcessor(hOngrid)
	processor := thrift.NewTMultiplexedProcessor()
	server := thrift.NewTSimpleServer4(processor, transport, transportFactory, protocolFactory)
	processor.RegisterProcessor("DB", dbProcessor)
	processor.RegisterProcessor("Ongrid", ongridProcessor)

	fmt.Println("Starting the ongrid-thrift server ver 0.1.3 on ", addr)
	return server.Serve()
}
