//go:build test

package app

import evmtypes "github.com/cosmos/evm/x/vm/types"

// resetEVMConfig clears cosmos/evm's process-global chain config, coin info, and
// EIP activators so that more than one ChainApp can be constructed within a
// single test binary (cosmos/evm otherwise seals these the first time and panics
// on a second app). Compiled only under the `test` build tag, where cosmos/evm's
// resettable config variant is active; a no-op in production builds.
func resetEVMConfig() {
	evmtypes.NewEVMConfigurator().ResetTestConfig()
}
