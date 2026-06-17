package types_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// valid tsc addresses (the default params' addresses are well-formed).
var (
	addrA = types.DefaultMintingAddress
	addrB = types.DefaultReceivingAddress
)

func TestMsgClaimValidateBasic(t *testing.T) {
	base := func() *types.MsgClaim {
		return &types.MsgClaim{
			Claimer:    addrA,
			Date:       "2025-07-22",
			DistroType: "type1",
			Nonce:      0,
			Address:    addrB,
			Category:   "type1",
			Amount:     "3000",
			Denom:      "aTSC",
			Proof:      [][]byte{[]byte("sibling")},
		}
	}

	require.NoError(t, base().ValidateBasic())

	cases := []struct {
		name   string
		mutate func(*types.MsgClaim)
		errSub string
	}{
		{"bad claimer", func(m *types.MsgClaim) { m.Claimer = "not-an-address" }, "invalid claimer"},
		{"bad reward address", func(m *types.MsgClaim) { m.Address = "not-an-address" }, "invalid reward address"},
		{"empty date", func(m *types.MsgClaim) { m.Date = "" }, "date cannot be empty"},
		{"malformed date", func(m *types.MsgClaim) { m.Date = "2025/07/22" }, "YYYY-MM-DD"},
		{"empty distro type", func(m *types.MsgClaim) { m.DistroType = "" }, "distro_type cannot be empty"},
		{"empty category", func(m *types.MsgClaim) { m.Category = "" }, "category cannot be empty"},
		{"empty denom", func(m *types.MsgClaim) { m.Denom = "" }, "denom cannot be empty"},
		{"non-integer amount", func(m *types.MsgClaim) { m.Amount = "abc" }, "amount is not a valid integer"},
		{"non-positive amount", func(m *types.MsgClaim) { m.Amount = "0" }, "amount must be positive"},
		{"proof too long", func(m *types.MsgClaim) { m.Proof = make([][]byte, types.MaxProofDepth+1) }, "merkle proof too long"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := base()
			tc.mutate(m)
			err := m.ValidateBasic()
			require.Error(t, err)
			require.ErrorContains(t, err, tc.errSub)
		})
	}

	// A proof exactly at the maximum depth is accepted.
	m := base()
	m.Proof = make([][]byte, types.MaxProofDepth)
	require.NoError(t, m.ValidateBasic())
}

func TestMsgSubmitDistributionRootValidateBasic(t *testing.T) {
	valid := func() *types.MsgSubmitDistributionRoot {
		return &types.MsgSubmitDistributionRoot{
			Signer:           addrA,
			Date:             "2025-07-22",
			DistroType:       "type1",
			MerkleRoot:       []byte("root"),
			Version:          1,
			TotalsByCategory: map[string]string{"type1": "1000"},
			HeaderProof:      [][]byte{[]byte("sibling")},
		}
	}
	require.NoError(t, valid().ValidateBasic())

	bad := valid()
	bad.Signer = "bad"
	require.ErrorContains(t, bad.ValidateBasic(), "invalid signer")

	bad = valid()
	bad.Date = "bad"
	require.ErrorContains(t, bad.ValidateBasic(), "YYYY-MM-DD")

	bad = valid()
	bad.DistroType = ""
	require.ErrorContains(t, bad.ValidateBasic(), "distro_type cannot be empty")

	bad = valid()
	bad.MerkleRoot = nil
	require.ErrorContains(t, bad.ValidateBasic(), "merkle root cannot be empty")

	bad = valid()
	bad.TotalsByCategory = nil
	require.ErrorContains(t, bad.ValidateBasic(), "totals_by_category must not be empty")

	bad = valid()
	bad.TotalsByCategory = map[string]string{"type1": "x"}
	require.ErrorContains(t, bad.ValidateBasic(), "is not a valid integer")

	bad = valid()
	bad.HeaderProof = make([][]byte, types.MaxProofDepth+1)
	require.ErrorContains(t, bad.ValidateBasic(), "header proof too long")
}

func TestMsgChallengeDistributionValidateBasic(t *testing.T) {
	require.NoError(t, (&types.MsgChallengeDistribution{Challenger: addrA, Date: "2025-07-22", DistroType: "type1"}).ValidateBasic())
	require.ErrorContains(t, (&types.MsgChallengeDistribution{Challenger: "bad", Date: "2025-07-22", DistroType: "type1"}).ValidateBasic(), "invalid challenger")
	require.ErrorContains(t, (&types.MsgChallengeDistribution{Challenger: addrA, Date: "2025-07-22"}).ValidateBasic(), "distro_type cannot be empty")
	require.ErrorContains(t, (&types.MsgChallengeDistribution{Challenger: addrA, Date: "", DistroType: "type1"}).ValidateBasic(), "date cannot be empty")
}

func TestMsgReviveDistributionValidateBasic(t *testing.T) {
	require.NoError(t, (&types.MsgReviveDistribution{Authority: addrA, Date: "2025-07-22", DistroType: "type1"}).ValidateBasic())
	require.ErrorContains(t, (&types.MsgReviveDistribution{Authority: "bad", Date: "2025-07-22", DistroType: "type1"}).ValidateBasic(), "invalid authority")
	require.ErrorContains(t, (&types.MsgReviveDistribution{Authority: addrA, Date: "2025-07-22"}).ValidateBasic(), "distro_type cannot be empty")
	require.ErrorContains(t, (&types.MsgReviveDistribution{Authority: addrA, Date: "nope", DistroType: "type1"}).ValidateBasic(), "YYYY-MM-DD")
}

// TestParamsRejectsZeroReviewDelay covers L2: a zero review delay removes the
// challenge window and must be rejected.
func TestParamsRejectsZeroReviewDelay(t *testing.T) {
	p := types.DefaultParams()
	require.NoError(t, p.Validate())

	p.ReviewDelayDays = 0
	err := p.Validate()
	require.Error(t, err)
	require.ErrorContains(t, err, "review delay days must be greater than zero")
}
