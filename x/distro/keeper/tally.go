package keeper

import (
	"context"
	"sort"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	licensetypes "github.com/webstack-sdk/webstack/x/licenses/types"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// licenseStatusActive is the x/licenses status string for an active license.
const licenseStatusActive = "active"

// maxDelegationsPerVoter caps how many of a delegator's delegations we sum when
// computing stake weight. Validators are limited to far fewer delegations than
// this in practice.
const maxDelegationsPerVoter = uint16(65535)

// activeLicenseCount returns how many active licenses of typeID holder owns.
func (k Keeper) activeLicenseCount(ctx context.Context, holder, typeID string) (uint64, error) {
	resp, err := k.licensesKeeper.LicensesByHolderAndType(ctx, &licensetypes.QueryLicensesByHolderAndTypeRequest{
		Holder: holder,
		TypeId: typeID,
	})
	if err != nil {
		return 0, err
	}
	var n uint64
	for _, l := range resp.Licenses {
		if l.Status == licenseStatusActive {
			n++
		}
	}
	return n, nil
}

// totalActiveLicensesOfType returns the network-wide count of active licenses of
// typeID. This is the denominator of the license-based tally.
func (k Keeper) totalActiveLicensesOfType(ctx context.Context, typeID string) (uint64, error) {
	resp, err := k.licensesKeeper.LicensesByType(ctx, &licensetypes.QueryLicensesByTypeRequest{TypeId: typeID})
	if err != nil {
		return 0, err
	}
	var n uint64
	for _, l := range resp.Licenses {
		if l.Status == licenseStatusActive {
			n++
		}
	}
	return n, nil
}

// voterStake returns the bonded stake weight of a voting address. A
// validator-operator address contributes only its self-delegation; any other
// address contributes the sum of its own delegations.
func (k Keeper) voterStake(ctx context.Context, addr sdk.AccAddress) (math.Int, error) {
	valAddr := sdk.ValAddress(addr)

	// If the address operates a validator, count only its self-delegation.
	if validator, err := k.stakingKeeper.GetValidator(ctx, valAddr); err == nil {
		del, err := k.stakingKeeper.GetDelegation(ctx, addr, valAddr)
		if err != nil {
			// Validator with no self-delegation contributes nothing.
			return math.ZeroInt(), nil
		}
		return validator.TokensFromShares(del.Shares).TruncateInt(), nil
	}

	// Otherwise sum the tokens backing all of this delegator's delegations.
	dels, err := k.stakingKeeper.GetDelegatorDelegations(ctx, addr, maxDelegationsPerVoter)
	if err != nil {
		return math.ZeroInt(), err
	}
	total := math.ZeroInt()
	for _, del := range dels {
		valAddr, err := sdk.ValAddressFromBech32(del.ValidatorAddress)
		if err != nil {
			return math.ZeroInt(), err
		}
		validator, err := k.stakingKeeper.GetValidator(ctx, valAddr)
		if err != nil {
			// Validator no longer exists; skip.
			continue
		}
		total = total.Add(validator.TokensFromShares(del.Shares).TruncateInt())
	}
	return total, nil
}

// tallyResult is a winning root and the fractions it achieved.
type tallyResult struct {
	root         []byte
	licenseTally math.LegacyDec
	stakeTally   math.LegacyDec
}

// tallyEpoch groups all votes for epoch by submitted root, computes the
// license-weighted and stake-weighted fractions for each, and returns the
// winning root if any passes BOTH configured thresholds. Returns (nil, nil) if
// no root reaches consensus. Iteration is deterministic (sorted by root bytes).
func (k Keeper) tallyEpoch(ctx context.Context, epoch int64, params types.Params) (*tallyResult, error) {
	type group struct {
		licenseWeight uint64
		stakeWeight   math.Int
	}
	groups := map[string]*group{}

	rng := collections.NewPrefixedPairRange[int64, string](epoch)
	err := k.Votes.Walk(ctx, rng, func(key collections.Pair[int64, string], vote types.DistributionVote) (bool, error) {
		signer := key.K2()

		// A vote only counts while its signer still holds an active license.
		lc, err := k.activeLicenseCount(ctx, signer, params.DistributionLicenseTypeId)
		if err != nil {
			return true, err
		}
		if lc == 0 {
			return false, nil
		}

		accAddr, err := sdk.AccAddressFromBech32(signer)
		if err != nil {
			return true, err
		}
		stake, err := k.voterStake(ctx, accAddr)
		if err != nil {
			return true, err
		}

		rk := string(vote.MerkleRoot)
		g := groups[rk]
		if g == nil {
			g = &group{stakeWeight: math.ZeroInt()}
			groups[rk] = g
		}
		g.licenseWeight += lc
		g.stakeWeight = g.stakeWeight.Add(stake)
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return nil, nil
	}

	totalLicenses, err := k.totalActiveLicensesOfType(ctx, params.DistributionLicenseTypeId)
	if err != nil {
		return nil, err
	}
	totalBonded, err := k.stakingKeeper.TotalBondedTokens(ctx)
	if err != nil {
		return nil, err
	}
	licenseThreshold, err := math.LegacyNewDecFromStr(params.LicenseTallyThreshold)
	if err != nil {
		return nil, err
	}
	stakeThreshold, err := math.LegacyNewDecFromStr(params.StakeTallyThreshold)
	if err != nil {
		return nil, err
	}

	// Evaluate roots in a deterministic order so the outcome is identical on
	// every node even if multiple roots could clear a (sub-majority) threshold.
	roots := make([]string, 0, len(groups))
	for rk := range groups {
		roots = append(roots, rk)
	}
	sort.Strings(roots)

	for _, rk := range roots {
		g := groups[rk]
		licenseFrac := uintFraction(g.licenseWeight, totalLicenses)
		stakeFrac := intFraction(g.stakeWeight, totalBonded)
		if licenseFrac.GTE(licenseThreshold) && stakeFrac.GTE(stakeThreshold) {
			return &tallyResult{
				root:         []byte(rk),
				licenseTally: licenseFrac,
				stakeTally:   stakeFrac,
			}, nil
		}
	}
	return nil, nil
}

func uintFraction(num, den uint64) math.LegacyDec {
	if den == 0 {
		return math.LegacyZeroDec()
	}
	return math.LegacyNewDec(int64(num)).Quo(math.LegacyNewDec(int64(den)))
}

func intFraction(num, den math.Int) math.LegacyDec {
	if !den.IsPositive() {
		return math.LegacyZeroDec()
	}
	return math.LegacyNewDecFromInt(num).Quo(math.LegacyNewDecFromInt(den))
}
