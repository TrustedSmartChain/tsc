//go:build test

package app

import (
	evmtypes "github.com/cosmos/evm/x/vm/types"
)

// resetEVMChainConfig clears cosmos/evm's process-global chain config so each
// NewChainApp in a test binary can configure it fresh. Only available (and
// only needed) under -tags test — the tag also selects cosmos/evm's
// mutex-guarded test config implementation.
func resetEVMChainConfig() {
	evmtypes.NewEVMConfigurator().ResetTestConfig()
}
