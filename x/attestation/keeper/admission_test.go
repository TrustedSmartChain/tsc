package keeper_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	networktypes "github.com/webstack-sdk/webstack/x/network/types"

	"github.com/TrustedSmartChain/tsc/v3/x/attestation/types"
)

// TestAdmissionQuotaBurn: every admitted attempt burns the per-node daily
// counter — including ones whose handler would later fail — and the counter
// resets across UTC days.
func TestAdmissionQuotaBurn(t *testing.T) {
	f := SetupTest(t)
	node := f.addNode(types.NodeTypeTrust, networktypes.NodeActive)
	msg := &types.MsgAttestRwa{NodeAddress: node.Address, Attestations: attestations(1)}

	params, err := f.k.Params.Get(f.ctx)
	require.NoError(t, err)

	for i := uint64(0); i < params.RwaDailyLimit; i++ {
		require.NoError(t, f.k.CheckAndConsumeGaslessQuota(f.ctx, msg))
	}
	require.ErrorIs(t, f.k.CheckAndConsumeGaslessQuota(f.ctx, msg), types.ErrQuotaExceeded)

	// RWA and RWU quotas are independent.
	require.NoError(t, f.k.CheckAndConsumeGaslessQuota(f.ctx, &types.MsgAttestRwu{
		NodeAddress:  node.Address,
		Attestations: attestations(1),
	}))

	// The counter resets on the next UTC day.
	nextDay := f.WithBlockTime(fixtureBlockTime.Add(24 * time.Hour))
	require.NoError(t, f.k.CheckAndConsumeGaslessQuota(nextDay, msg))
}

// TestAdmissionLicenseGate: the operator's license standing is checked on
// the counter's first roll of each UTC day, not on every tx.
func TestAdmissionLicenseGate(t *testing.T) {
	f := SetupTest(t)
	node := f.addNode(types.NodeTypeTrust, networktypes.NodeActive)
	msg := &types.MsgAttestRwa{NodeAddress: node.Address, Attestations: attestations(1)}

	// First tx of the day passes and consumes quota.
	require.NoError(t, f.k.CheckAndConsumeGaslessQuota(f.ctx, msg))

	// Licenses revoked mid-day: same-day txs still pass (the gate checks on
	// roll)...
	f.network.unlicensed[node.Operator] = true
	require.NoError(t, f.k.CheckAndConsumeGaslessQuota(f.ctx, msg))

	// ...but the first tx of the next day is rejected: a revoked operator's
	// fleet does not keep its free tx channel.
	nextDay := f.WithBlockTime(fixtureBlockTime.Add(24 * time.Hour))
	require.ErrorIs(t, f.k.CheckAndConsumeGaslessQuota(nextDay, msg), networktypes.ErrNoActiveLicenses)
}

func TestAdmissionStanding(t *testing.T) {
	f := SetupTest(t)

	// Unknown node.
	require.ErrorIs(t, f.k.CheckAndConsumeGaslessQuota(f.ctx, &types.MsgAttestRwa{
		NodeAddress:  "unknown",
		Attestations: attestations(1),
	}), types.ErrNodeNotFound)

	// Deactivated node.
	deactivated := f.addNode(types.NodeTypeTrust, networktypes.NodeDeactivated)
	require.ErrorIs(t, f.k.CheckAndConsumeGaslessQuota(f.ctx, &types.MsgAttestRwa{
		NodeAddress:  deactivated.Address,
		Attestations: attestations(1),
	}), types.ErrNodeNotActive)

	// Non-attestation msgs fail closed.
	require.ErrorIs(t, f.k.CheckAndConsumeGaslessQuota(f.ctx, &types.MsgUpdateParams{}), types.ErrNotGaslessMsg)
}
