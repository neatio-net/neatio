package node

import (
	"reflect"

	"github.com/nio-net/nio/chain/log"
	"github.com/nio-net/nio/network/p2p"
	"github.com/nio-net/nio/network/rpc"
)

func (n *Node) RpcAPIs() []rpc.API {
	return n.rpcAPIs
}

func (n *Node) Start1() error {
	n.lock.Lock()
	defer n.lock.Unlock()

	if err := n.openDataDir(); err != nil {
		return err
	}

	if n.server == nil {
		return ErrNodeStopped
	}

	services := n.services

	running := n.server

	started := []reflect.Type{}
	for kind, service := range services {

		if err := service.Start(running); err != nil {
			for _, kind := range started {
				services[kind].Stop()
			}
			running.Stop()

			return err
		}

		started = append(started, kind)
	}

	if err := n.startRPC1(services); err != nil {
		for _, service := range services {
			service.Stop()
		}
		running.Stop()
		return err
	}

	n.services = services
	n.server = running
	n.stop = make(chan struct{})

	return nil
}

func (n *Node) GatherServices() error {

	services := make(map[reflect.Type]Service)
	for _, constructor := range n.serviceFuncs {

		ctx := &ServiceContext{
			config:         n.config,
			services:       make(map[reflect.Type]Service),
			EventMux:       n.eventmux,
			AccountManager: n.accman,
		}
		for kind, s := range services {
			ctx.services[kind] = s
		}

		service, err := constructor(ctx)
		if err != nil {
			return err
		}
		kind := reflect.TypeOf(service)
		if _, exists := services[kind]; exists {
			return &DuplicateServiceError{Kind: kind}
		}
		services[kind] = service
	}

	n.services = services

	return nil
}

func (n *Node) GatherProtocols() []p2p.Protocol {

	protocols := make([]p2p.Protocol, 0)
	for _, service := range n.services {
		protocols = append(protocols, service.Protocols()...)
	}

	return protocols
}

func (n *Node) GetHTTPHandler() (*rpc.Server, error) {

	whitelist := make(map[string]bool)
	for _, module := range n.config.HTTPModules {
		whitelist[module] = true
	}

	handler := rpc.NewServer()
	for _, api := range n.rpcAPIs {
		if whitelist[api.Namespace] || (len(whitelist) == 0 && api.Public) {
			if err := handler.RegisterName(api.Namespace, api.Service); err != nil {
				return nil, err
			}
			n.log.Debug("HTTP registered", "service", api.Service, "namespace", api.Namespace)
		}
	}

	n.httpEndpoint = ""
	n.httpListener = nil
	n.httpHandler = nil

	return handler, nil
}

func (n *Node) GetWSHandler() (*rpc.Server, error) {

	whitelist := make(map[string]bool)
	for _, module := range n.config.WSModules {
		whitelist[module] = true
	}

	handler := rpc.NewServer()
	for _, api := range n.rpcAPIs {
		if n.config.WSExposeAll || whitelist[api.Namespace] || (len(whitelist) == 0 && api.Public) {
			if err := handler.RegisterName(api.Namespace, api.Service); err != nil {
				return nil, err
			}
			log.Debug("WebSocket registered", "service", api.Service, "namespace", api.Namespace)
		}
	}

	n.wsEndpoint = ""
	n.wsListener = nil
	n.wsHandler = nil

	return handler, nil
}

func (n *Node) startRPC1(services map[reflect.Type]Service) error {

	apis := n.apis()
	for _, service := range services {
		apis = append(apis, service.APIs()...)
	}

	if err := n.startInProc(apis); err != nil {
		return err
	}
	if err := n.startIPC(apis); err != nil {
		n.stopInProc()
		return err
	}

	n.rpcAPIs = apis
	return nil
}

func (n *Node) GetLogger() log.Logger {
	return n.log
}
