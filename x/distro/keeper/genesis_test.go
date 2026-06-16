package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// fullGenesis builds a populated, self-consistent genesis state covering every
// collection: votes, distributions in several lifecycle states, claimed reward
// nonces, and per-category claim totals.
func fullGenesis() *types.GenesisState {
	const (
		signerA    = "tsc1signerA"
		signerB    = "tsc1signerB"
		challenger = "tsc1challenger"
	)
	return &types.GenesisState{
		Params: types.DefaultParams(),
		Votes: []types.DistributionVote{
			{Date: dateOf(1), DistroType: defaultType, Signer: signerA, MerkleRoot: []byte("root-day-1-signerA-padded-to-32!")},
			{Date: dateOf(3), DistroType: defaultType, Signer: signerB, MerkleRoot: []byte("root-day-3-signerB-padded-to-32!")},
		},
		Distributions: []types.Distribution{
			{
				Date:            dateOf(1),
				DistroType:      defaultType,
				MerkleRoot:      []byte("canonical-day-1-root-padded-32!!"),
				Status:          types.DISTRIBUTION_STATUS_LIVE,
				LicenseTally:    "0.75",
				StakeTally:      "0.70",
				FinalizedHeight: 42,
				ClaimedAmount:   "1000",
			},
			{
				Date:             dateOf(2),
				DistroType:       defaultType,
				MerkleRoot:       []byte("canonical-day-2-root-padded-32!!"),
				Status:           types.DISTRIBUTION_STATUS_PENDING,
				LicenseTally:     "0.80",
				StakeTally:       "0.72",
				PendingSinceDate: dateOf(2),
			},
			{
				Date:       dateOf(3),
				DistroType: defaultType,
				Status:     types.DISTRIBUTION_STATUS_VOTING,
			},
			{
				Date:          dateOf(4),
				DistroType:    defaultType,
				MerkleRoot:    []byte("canonical-day-4-root-padded-32!!"),
				Status:        types.DISTRIBUTION_STATUS_UNDER_REVIEW,
				LicenseTally:  "0.90",
				StakeTally:    "0.85",
				Challenger:    challenger,
				ChallengeBond: "100",
			},
		},
		// Only the LIVE day may carry claimed rewards. Nonces ascending to match
		// the export order.
		ClaimedRewards: []types.ClaimedReward{
			{Date: dateOf(1), DistroType: defaultType, Nonces: []uint64{0, 1, 2}},
		},
		CategoryClaimTotals: []types.CategoryClaimTotal{
			{Date: dateOf(1), DistroType: defaultType, Category: "type1", Total: "600"},
			{Date: dateOf(1), DistroType: defaultType, Category: "type2", Total: "400"},
		},
	}
}

func TestGenesisRoundTrip(t *testing.T) {
	f := SetupTest(t)
	gs := fullGenesis()

	// The fixture genesis must itself be valid.
	require.NoError(t, gs.Validate())

	require.NoError(t, f.k.InitGenesis(f.ctx, gs))
	got := f.k.ExportGenesis(f.ctx)
	require.NotNil(t, got)

	require.Equal(t, gs.Params, got.Params)
	require.ElementsMatch(t, gs.Votes, got.Votes)
	require.ElementsMatch(t, gs.Distributions, got.Distributions)
	require.ElementsMatch(t, gs.ClaimedRewards, got.ClaimedRewards)
	require.ElementsMatch(t, gs.CategoryClaimTotals, got.CategoryClaimTotals)

	// A second round trip from the exported state must be stable.
	f2 := SetupTest(t)
	require.NoError(t, f2.k.InitGenesis(f2.ctx, got))
	got2 := f2.k.ExportGenesis(f2.ctx)
	require.Equal(t, got, got2)
}

func TestGenesisValidate(t *testing.T) {
	require.NoError(t, fullGenesis().Validate(), "the base fixture must be valid")

	tests := []struct {
		name   string
		mutate func(gs *types.GenesisState)
		errStr string
	}{
		{
			name: "duplicate distribution date",
			mutate: func(gs *types.GenesisState) {
				gs.Distributions = append(gs.Distributions, types.Distribution{
					Date:       dateOf(1),
					DistroType: defaultType,
					Status:     types.DISTRIBUTION_STATUS_VOTING,
				})
			},
			errStr: "duplicate distribution",
		},
		{
			name: "pending without root",
			mutate: func(gs *types.GenesisState) {
				gs.Distributions[1].MerkleRoot = nil // dateOf(2) is PENDING
			},
			errStr: "empty merkle root",
		},
		{
			name: "under review without challenger",
			mutate: func(gs *types.GenesisState) {
				gs.Distributions[3].Challenger = "" // dateOf(4) is UNDER_REVIEW
			},
			errStr: "no challenger",
		},
		{
			name: "claimed rewards reference unknown date",
			mutate: func(gs *types.GenesisState) {
				gs.ClaimedRewards[0].Date = dateOf(99)
			},
			errStr: "unknown date",
		},
		{
			name: "claimed rewards reference non-live date",
			mutate: func(gs *types.GenesisState) {
				gs.ClaimedRewards[0].Date = dateOf(2) // PENDING, not LIVE
			},
			errStr: "non-live",
		},
		{
			name: "invalid challenge bond",
			mutate: func(gs *types.GenesisState) {
				gs.Distributions[3].ChallengeBond = "not-a-number"
			},
			errStr: "invalid challenge bond",
		},
		{
			name: "invalid claimed amount",
			mutate: func(gs *types.GenesisState) {
				gs.Distributions[0].ClaimedAmount = "not-a-number"
			},
			errStr: "invalid claimed amount",
		},
		{
			name: "vote with empty root",
			mutate: func(gs *types.GenesisState) {
				gs.Votes[0].MerkleRoot = nil
			},
			errStr: "empty merkle root",
		},
		{
			name: "invalid params",
			mutate: func(gs *types.GenesisState) {
				gs.Params.VoteWindowDays = 0
			},
			errStr: "vote window days",
		},
		{
			name: "malformed distribution date",
			mutate: func(gs *types.GenesisState) {
				gs.Distributions[2].Date = "2025/07/24" // dateOf(3), wrong format
			},
			errStr: "YYYY-MM-DD",
		},
		{
			name: "invalid voting_since_date",
			mutate: func(gs *types.GenesisState) {
				gs.Distributions[2].VotingSinceDate = "nope" // dateOf(3) is VOTING
			},
			errStr: "voting_since_date",
		},
		{
			name: "category total references unknown date",
			mutate: func(gs *types.GenesisState) {
				gs.CategoryClaimTotals[0].Date = dateOf(99)
			},
			errStr: "unknown date",
		},
		{
			name: "category total references non-live date",
			mutate: func(gs *types.GenesisState) {
				gs.CategoryClaimTotals[0].Date = dateOf(3) // VOTING, not LIVE
			},
			errStr: "non-live",
		},
		{
			name: "category total with invalid amount",
			mutate: func(gs *types.GenesisState) {
				gs.CategoryClaimTotals[0].Total = "not-a-number"
			},
			errStr: "invalid total",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gs := fullGenesis()
			tc.mutate(gs)
			err := gs.Validate()
			require.Error(t, err)
			require.ErrorContains(t, err, tc.errStr)
		})
	}
}
