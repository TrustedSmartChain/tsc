//go:build !test

package app

// resetEVMChainConfig is a no-op in production builds. cosmos/evm v0.7 makes
// the global EVM chain config set-once per process: a second NewChainApp with
// a non-default chain id panics. That is fine for the real binary (temp app
// uses the default id, the real app sets ours exactly once), but test binaries
// construct many apps, so the test-tagged variant of this function resets the
// global between constructions. Run tests with -tags test (see Makefile).
func resetEVMChainConfig() {}
