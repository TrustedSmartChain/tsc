//go:build !test

package app

// resetEVMConfig is a no-op outside the `test` build tag. See the test-tagged
// variant for details.
func resetEVMConfig() {}
