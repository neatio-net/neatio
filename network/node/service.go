package node

import (
	"crypto/ecdsa"
	"reflect"

	"github.com/neatlab/neatio/chain/core/rawdb"

	"github.com/neatlab/neatio/chain/accounts"
	"github.com/neatlab/neatio/neatdb"
	"github.com/neatlab/neatio/network/p2p"
	"github.com/neatlab/neatio/network/rpc"
	"github.com/neatlab/neatio/utilities/event"
)

type ServiceContext struct {
	config         *Config
	services       map[reflect.Type]Service
	EventMux       *event.TypeMux
	AccountManager *accounts.Manager
}

func (ctx *ServiceContext) OpenDatabase(name string, cache int, handles int, namespace string) (neatdb.Database, error) {
	if ctx.config.DataDir == "" {
		return rawdb.NewMemoryDatabase(), nil
	}
	db, err := rawdb.NewLevelDBDatabase(ctx.config.ResolvePath(name), cache, handles, namespace)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func (ctx *ServiceContext) ResolvePath(path string) string {
	return ctx.config.ResolvePath(path)
}

func (ctx *ServiceContext) Service(service interface{}) error {
	element := reflect.ValueOf(service).Elem()
	if running, ok := ctx.services[element.Type()]; ok {
		element.Set(reflect.ValueOf(running))
		return nil
	}
	return ErrServiceUnknown
}

func (ctx *ServiceContext) NodeKey() *ecdsa.PrivateKey {
	return ctx.config.NodeKey()
}

func (ctx *ServiceContext) ChainId() string {
	return ctx.config.ChainId
}

type ServiceConstructor func(ctx *ServiceContext) (Service, error)

type Service interface {
	Protocols() []p2p.Protocol

	APIs() []rpc.API

	Start(server *p2p.Server) error

	Stop() error
}
