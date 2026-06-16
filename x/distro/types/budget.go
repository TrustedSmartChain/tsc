package types

import "cosmossdk.io/math"

// Budget split helpers for per-type distributions. These are pure (no keeper,
// store, or context) so the off-chain worker that computes daily distributions
// can import x/distro/types and reproduce the chain's per-type and per-category
// caps byte-for-byte. They build on DateBudget (the halving per-day cap).

// TypeBudget returns the portion of a day's budget a distribution type may claim:
//
//	floor(dayBudget * typePercentage)
//
// typePercentage is a decimal fraction string in [0,1]; an empty or non-positive
// string yields zero.
func TypeBudget(dayBudget math.Int, typePercentage string) math.Int {
	return scaleByFraction(dayBudget, typePercentage)
}

// CategoryCap returns the portion of a type's budget a category may claim:
//
//	floor(typeBudget * categoryPercentage)
//
// An empty or non-positive percentage yields zero. When a type has no category
// percentages at all (DistributionType.HasCategoryPercentages is false) callers
// treat its categories as uncapped within the type budget rather than capping
// each at zero.
func CategoryCap(typeBudget math.Int, categoryPercentage string) math.Int {
	return scaleByFraction(typeBudget, categoryPercentage)
}

func scaleByFraction(amount math.Int, fraction string) math.Int {
	if fraction == "" {
		return math.ZeroInt()
	}
	dec, err := math.LegacyNewDecFromStr(fraction)
	if err != nil || !dec.IsPositive() {
		return math.ZeroInt()
	}
	return math.LegacyNewDecFromInt(amount).Mul(dec).TruncateInt()
}
