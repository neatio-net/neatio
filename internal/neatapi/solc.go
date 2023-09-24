package neatapi

import (
	"sync"

	"github.com/nio-net/nio/network/rpc"
	"github.com/nio-net/nio/utilities/common/compiler"
)

func makeCompilerAPIs(solcPath string) []rpc.API {
	c := &compilerAPI{solc: solcPath}
	return []rpc.API{
		{
			Namespace: "eth",
			Version:   "1.0",
			Service:   (*PublicCompilerAPI)(c),
			Public:    true,
		},
		{
			Namespace: "neat",
			Version:   "1.0",
			Service:   (*PublicCompilerAPI)(c),
			Public:    true,
		},
		{
			Namespace: "admin",
			Version:   "1.0",
			Service:   (*CompilerAdminAPI)(c),
			Public:    true,
		},
	}
}

type compilerAPI struct {
	mu   sync.Mutex
	solc string
}

type CompilerAdminAPI compilerAPI

func (api *CompilerAdminAPI) SetSolc(path string) (string, error) {
	api.mu.Lock()
	defer api.mu.Unlock()
	info, err := compiler.SolidityVersion(path)
	if err != nil {
		return "", err
	}
	api.solc = path
	return info.FullVersion, nil
}

type PublicCompilerAPI compilerAPI

func (api *PublicCompilerAPI) CompileSolidity(source string) (map[string]*compiler.Contract, error) {
	api.mu.Lock()
	defer api.mu.Unlock()
	return compiler.CompileSolidityString(api.solc, source)
}

func (api *PublicCompilerAPI) GetCompilers() ([]string, error) {
	api.mu.Lock()
	defer api.mu.Unlock()
	if _, err := compiler.SolidityVersion(api.solc); err == nil {
		return []string{"Solidity"}, nil
	}
	return []string{}, nil
}
