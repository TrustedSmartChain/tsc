package types_test

import (
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// twoType returns a valid params with two distribution types (60% / 40%), the
// first with capped categories (50/50) and the second with uncapped categories.
func twoTypeParams() types.Params {
	p := types.DefaultParams()
	p.DistributionTypes = []types.DistributionType{
		{
			Id:         "type1",
			Percentage: "0.6",
			Categories: []types.DistributionCategory{
				{Name: "a", Percentage: "0.5"},
				{Name: "b", Percentage: "0.5"},
			},
			LicenseTallyThreshold: "0.667",
			StakeTallyThreshold:   "0.667",
		},
		{
			Id:         "type2",
			Percentage: "0.4",
			Categories: []types.DistributionCategory{
				{Name: "c", Percentage: ""},
				{Name: "d", Percentage: ""},
			},
			ValidatorTallyThreshold: "0.667",
		},
	}
	return p
}

func TestParamsDistributionTypesValid(t *testing.T) {
	require.NoError(t, twoTypeParams().Validate())
}

func TestParamsTypePercentagesMustSumToOne(t *testing.T) {
	p := twoTypeParams()
	p.DistributionTypes[1].Percentage = "0.3" // 0.6 + 0.3 = 0.9
	require.ErrorContains(t, p.Validate(), "type percentages must sum to exactly 1")
}

func TestParamsCategoryPercentagesAllOrNothing(t *testing.T) {
	// Some-but-not-all categories carrying a percentage is rejected: once any is
	// set they must sum to exactly 1.
	p := twoTypeParams()
	p.DistributionTypes[0].Categories[1].Percentage = "" // a=0.5, b=unset -> sum 0.5
	require.ErrorContains(t, p.Validate(), "category percentages must sum to exactly 1")
}

func TestParamsCategoriesUncappedIsValid(t *testing.T) {
	p := twoTypeParams()
	// type2 already has all-empty category percentages (uncapped); it must pass.
	require.NoError(t, p.Validate())
	require.False(t, p.DistributionTypes[1].HasCategoryPercentages())
	require.True(t, p.DistributionTypes[0].HasCategoryPercentages())
}

func TestParamsTypeRequiresAConsensusMechanism(t *testing.T) {
	p := twoTypeParams()
	p.DistributionTypes[0].LicenseTallyThreshold = ""
	p.DistributionTypes[0].StakeTallyThreshold = ""
	p.DistributionTypes[0].ValidatorTallyThreshold = ""
	require.ErrorContains(t, p.Validate(), "must configure at least one consensus threshold")
}

func TestParamsRejectsDuplicateType(t *testing.T) {
	p := twoTypeParams()
	p.DistributionTypes[1].Id = "type1"
	require.ErrorContains(t, p.Validate(), "duplicate distribution type")
}

func TestParamsValidatorAddresses(t *testing.T) {
	p := twoTypeParams()
	p.ValidatorAddresses = []string{types.DefaultMintingAddress, types.DefaultMintingAddress}
	require.ErrorContains(t, p.Validate(), "duplicate validator address")

	p.ValidatorAddresses = []string{"not-bech32"}
	require.ErrorContains(t, p.Validate(), "invalid validator address")
}

func TestTypeAndCategoryBudget(t *testing.T) {
	day := math.NewInt(1000)
	// 60% of 1000 = 600.
	typeBudget := types.TypeBudget(day, "0.6")
	require.Equal(t, math.NewInt(600), typeBudget)
	// 50% of 600 = 300.
	require.Equal(t, math.NewInt(300), types.CategoryCap(typeBudget, "0.5"))
	// Empty / non-positive fraction yields zero.
	require.True(t, types.TypeBudget(day, "").IsZero())
	require.True(t, types.CategoryCap(typeBudget, "0").IsZero())
}
