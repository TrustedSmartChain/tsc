package types_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// manyCategories builds a category map with n distinct entries (used to exceed
// the MaxCategories bound). The count check fires before the sum check, so the
// values are immaterial.
func manyCategories(n int) map[string]string {
	m := make(map[string]string, n)
	for i := 0; i < n; i++ {
		m["cat"+strconv.Itoa(i)] = "1"
	}
	return m
}

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
			Nonce:      0,
			Address:    addrB,
			Total:      "3000",
			Categories: map[string]string{"type1": "1000", "type2": "2000"},
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
		{"non-integer total", func(m *types.MsgClaim) { m.Total = "abc" }, "total is not a valid integer"},
		{"non-positive total", func(m *types.MsgClaim) { m.Total = "0" }, "total must be positive"},
		{"empty categories", func(m *types.MsgClaim) { m.Categories = nil }, "categories must not be empty"},
		{"empty category name", func(m *types.MsgClaim) { m.Categories = map[string]string{"": "3000"} }, "category name must not be empty"},
		{"bad category amount", func(m *types.MsgClaim) { m.Categories = map[string]string{"type1": "x"} }, "is not a valid integer"},
		{"non-positive category", func(m *types.MsgClaim) { m.Categories = map[string]string{"type1": "0"} }, "must be positive"},
		{"sum mismatch", func(m *types.MsgClaim) { m.Categories = map[string]string{"type1": "1"} }, "does not equal total"},
		{"proof too long", func(m *types.MsgClaim) { m.Proof = make([][]byte, types.MaxProofDepth+1) }, "merkle proof too long"},
		{"too many categories", func(m *types.MsgClaim) { m.Categories = manyCategories(types.MaxCategories + 1) }, "too many categories"},
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
	valid := &types.MsgSubmitDistributionRoot{Signer: addrA, Date: "2025-07-22", MerkleRoot: []byte("root")}
	require.NoError(t, valid.ValidateBasic())

	require.ErrorContains(t, (&types.MsgSubmitDistributionRoot{Signer: "bad", Date: "2025-07-22", MerkleRoot: []byte("r")}).ValidateBasic(), "invalid signer")
	require.ErrorContains(t, (&types.MsgSubmitDistributionRoot{Signer: addrA, Date: "bad", MerkleRoot: []byte("r")}).ValidateBasic(), "YYYY-MM-DD")
	require.ErrorContains(t, (&types.MsgSubmitDistributionRoot{Signer: addrA, Date: "2025-07-22"}).ValidateBasic(), "merkle root cannot be empty")
}

func TestMsgChallengeDistributionValidateBasic(t *testing.T) {
	require.NoError(t, (&types.MsgChallengeDistribution{Challenger: addrA, Date: "2025-07-22"}).ValidateBasic())
	require.ErrorContains(t, (&types.MsgChallengeDistribution{Challenger: "bad", Date: "2025-07-22"}).ValidateBasic(), "invalid challenger")
	require.ErrorContains(t, (&types.MsgChallengeDistribution{Challenger: addrA, Date: ""}).ValidateBasic(), "date cannot be empty")
}

func TestMsgReviveDistributionValidateBasic(t *testing.T) {
	require.NoError(t, (&types.MsgReviveDistribution{Authority: addrA, Date: "2025-07-22"}).ValidateBasic())
	require.ErrorContains(t, (&types.MsgReviveDistribution{Authority: "bad", Date: "2025-07-22"}).ValidateBasic(), "invalid authority")
	require.ErrorContains(t, (&types.MsgReviveDistribution{Authority: addrA, Date: "nope"}).ValidateBasic(), "YYYY-MM-DD")
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
