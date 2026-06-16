package keeper

import (
	"fmt"
	"strings"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// InvariantResult is the outcome of a single distro invariant check. It is the
// shared representation behind both the SDK invariants (exercised by the
// simulation runner) and the Query/Audit endpoint (so operators can verify
// module health on a live node, since no crisis module is wired for runtime
// checks).
type InvariantResult struct {
	Name    string
	Broken  bool
	Message string
}

// RegisterInvariants registers the module's invariants with the registry.
func RegisterInvariants(ir sdk.InvariantRegistry, k Keeper) {
	ir.RegisterRoute(types.ModuleName, claimBudgetInvariantName, ClaimBudgetInvariant(k))
	ir.RegisterRoute(types.ModuleName, bondSolvencyInvariantName, BondSolvencyInvariant(k))
}

const (
	claimBudgetInvariantName  = "claim-budget"
	bondSolvencyInvariantName = "bond-solvency"
)

// ClaimBudgetInvariant asserts that no day has minted more than its halving
// budget: for every day, cumulative ClaimedAmount <= dateBudget(date).
func ClaimBudgetInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		r := k.checkClaimBudget(ctx)
		return sdk.FormatInvariant(types.ModuleName, r.Name, r.Message), r.Broken
	}
}

// BondSolvencyInvariant asserts the module account holds at least the sum of all
// outstanding (UNDER_REVIEW) challenge bonds, i.e. every escrowed bond is fully
// backed and refundable.
func BondSolvencyInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		r := k.checkBondSolvency(ctx)
		return sdk.FormatInvariant(types.ModuleName, r.Name, r.Message), r.Broken
	}
}

// CheckInvariants runs every distro invariant and returns the per-check results.
func (k Keeper) CheckInvariants(ctx sdk.Context) []InvariantResult {
	return []InvariantResult{
		k.checkClaimBudget(ctx),
		k.checkBondSolvency(ctx),
	}
}

func (k Keeper) checkClaimBudget(ctx sdk.Context) InvariantResult {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return InvariantResult{Name: claimBudgetInvariantName, Broken: true, Message: fmt.Sprintf("failed to load params: %v", err)}
	}

	var violations strings.Builder
	count := 0
	err = k.Distributions.Walk(ctx, nil, func(key collections.Pair[string, string], d types.Distribution) (bool, error) {
		date := key.K1()
		distroType := key.K2()
		// Only (day, type)s that have been claimed against can violate the budget.
		if d.ClaimedAmount == "" {
			return false, nil
		}
		claimed, ok := math.NewIntFromString(d.ClaimedAmount)
		if !ok {
			count++
			fmt.Fprintf(&violations, "\tday %s type %s has unparseable claimed amount %q\n", date, distroType, d.ClaimedAmount)
			return false, nil
		}
		if claimed.IsNegative() {
			count++
			fmt.Fprintf(&violations, "\tday %s type %s has negative claimed amount %s\n", date, distroType, claimed)
			return false, nil
		}
		dayBudget, err := dateBudget(params, date)
		if err != nil {
			count++
			fmt.Fprintf(&violations, "\tday %s type %s budget error: %v\n", date, distroType, err)
			return false, nil
		}
		// The per-(day, type) cap is the day's halving budget scaled by the type's
		// configured percentage. A type no longer in params caps at zero (any prior
		// claim against it is a violation).
		budget := math.ZeroInt()
		if dt, ok := params.FindDistributionType(distroType); ok {
			budget = types.TypeBudget(dayBudget, dt.Percentage)
		}
		if claimed.GT(budget) {
			count++
			fmt.Fprintf(&violations, "\tday %s type %s claimed %s exceeds budget %s\n", date, distroType, claimed, budget)
		}
		return false, nil
	})
	if err != nil {
		return InvariantResult{Name: claimBudgetInvariantName, Broken: true, Message: fmt.Sprintf("failed to walk distributions: %v", err)}
	}

	if count == 0 {
		return InvariantResult{Name: claimBudgetInvariantName, Broken: false, Message: "all days within their halving budget"}
	}
	return InvariantResult{Name: claimBudgetInvariantName, Broken: true, Message: fmt.Sprintf("%d day(s) over budget:\n%s", count, violations.String())}
}

func (k Keeper) checkBondSolvency(ctx sdk.Context) InvariantResult {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return InvariantResult{Name: bondSolvencyInvariantName, Broken: true, Message: fmt.Sprintf("failed to load params: %v", err)}
	}

	outstanding := math.ZeroInt()
	var invalid strings.Builder
	hasInvalid := false
	err = k.Distributions.Walk(ctx, nil, func(key collections.Pair[string, string], d types.Distribution) (bool, error) {
		if d.Status != types.DISTRIBUTION_STATUS_UNDER_REVIEW || d.ChallengeBond == "" {
			return false, nil
		}
		bond, ok := math.NewIntFromString(d.ChallengeBond)
		if !ok || bond.IsNegative() {
			hasInvalid = true
			fmt.Fprintf(&invalid, "\tday %s type %s has invalid challenge bond %q\n", key.K1(), key.K2(), d.ChallengeBond)
			return false, nil
		}
		outstanding = outstanding.Add(bond)
		return false, nil
	})
	if err != nil {
		return InvariantResult{Name: bondSolvencyInvariantName, Broken: true, Message: fmt.Sprintf("failed to walk distributions: %v", err)}
	}

	moduleAddr := k.accountKeeper.GetModuleAddress(types.ModuleName)
	balance := k.bankKeeper.GetBalance(ctx, moduleAddr, params.Denom).Amount

	switch {
	case balance.LT(outstanding):
		return InvariantResult{Name: bondSolvencyInvariantName, Broken: true,
			Message: fmt.Sprintf("module insolvent: balance %s < outstanding bonds %s", balance, outstanding)}
	case hasInvalid:
		return InvariantResult{Name: bondSolvencyInvariantName, Broken: true,
			Message: fmt.Sprintf("invalid escrowed bond(s):\n%s", invalid.String())}
	default:
		return InvariantResult{Name: bondSolvencyInvariantName, Broken: false,
			Message: fmt.Sprintf("module balance %s covers outstanding bonds %s", balance, outstanding)}
	}
}
