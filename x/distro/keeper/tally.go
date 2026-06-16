package keeper

import (
	"context"
	"errors"
	"sort"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	licensetypes "github.com/webstack-sdk/webstack/x/licenses/types"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// maxDelegationsPerVoter caps how many of a delegator's delegations we sum when
// computing stake weight. Validators are limited to far fewer delegations than
// this in practice.
const maxDelegationsPerVoter = uint16(65535)

// licenseValidOn reports whether a license's validity window [start_date,
// end_date] contains day (all YYYY-MM-DD; an empty end_date is open-ended).
//
// Eligibility is judged by this historical window rather than the license's
// current status. The x/licenses module sets end_date to the revocation day when
// a license is revoked, so the window alone fully captures "was this license
// valid on day D" — including revocations. Crucially, a license issued after day
// D (start_date > D) does NOT count for day D, so a freshly minted license cannot
// retroactively vote on or challenge an earlier day's distribution.
func licenseValidOn(l licensetypes.License, day string) bool {
	if l.StartDate == "" || l.StartDate > day {
		return false
	}
	if l.EndDate != "" && l.EndDate < day {
		return false
	}
	return true
}

// licensesValidOnForHolder returns how many of holder's licenses of typeID were
// valid on day (YYYY-MM-DD). Used by the submit and challenge gates so a license
// minted after a given day cannot act on that day's distribution.
func (k Keeper) licensesValidOnForHolder(ctx context.Context, holder, typeID, day string) (uint64, error) {
	resp, err := k.licensesKeeper.LicensesByHolderAndType(ctx, &licensetypes.QueryLicensesByHolderAndTypeRequest{
		Holder: holder,
		TypeId: typeID,
	})
	if err != nil {
		return 0, err
	}
	var n uint64
	for _, l := range resp.Licenses {
		if licenseValidOn(l, day) {
			n++
		}
	}
	return n, nil
}

// tallyCache memoizes the external keeper reads that are invariant across a
// single epoch-end pass: the licenses of the distribution type, each voter's
// bonded stake, and the total bonded tokens. It MUST be created fresh per
// advanceDistributions call and never persisted — its validity relies on
// licenses and staking being immutable for the duration of the pass (the tally
// only reads them, and a bond burn/refund touches neither). The day-specific
// part — which licenses were valid on a given day — is a cheap in-memory filter
// (licenseValidOn) applied per day over the cached, day-agnostic license list,
// so the optimization survives the as-of-day-D eligibility rule.
type tallyCache struct {
	k      Keeper
	typeID string

	typeLicenses   []licensetypes.License
	licensesLoaded bool

	stakes      map[string]math.Int
	totalBonded *math.Int

	// validatorSet is the params validator-address allowlist for the count-based
	// validator consensus mechanism; validatorTotal is its size (the denominator).
	validatorSet   map[string]struct{}
	validatorTotal uint64
}

func newTallyCache(k Keeper, typeID string, validatorAddresses []string) *tallyCache {
	set := make(map[string]struct{}, len(validatorAddresses))
	for _, a := range validatorAddresses {
		set[a] = struct{}{}
	}
	return &tallyCache{
		k:              k,
		typeID:         typeID,
		stakes:         map[string]math.Int{},
		validatorSet:   set,
		validatorTotal: uint64(len(set)),
	}
}

// isValidator reports whether addr is in the configured validator allowlist.
func (c *tallyCache) isValidator(addr string) bool {
	_, ok := c.validatorSet[addr]
	return ok
}

func (c *tallyCache) loadLicenses(ctx context.Context) error {
	if c.licensesLoaded {
		return nil
	}
	resp, err := c.k.licensesKeeper.LicensesByType(ctx, &licensetypes.QueryLicensesByTypeRequest{TypeId: c.typeID})
	if err != nil {
		return err
	}
	c.typeLicenses = resp.Licenses
	c.licensesLoaded = true
	return nil
}

// licensesValidOn counts holder's licenses (of the cached type) valid on day.
func (c *tallyCache) licensesValidOn(ctx context.Context, holder, day string) (uint64, error) {
	if err := c.loadLicenses(ctx); err != nil {
		return 0, err
	}
	var n uint64
	for _, l := range c.typeLicenses {
		if l.Holder == holder && licenseValidOn(l, day) {
			n++
		}
	}
	return n, nil
}

// totalValidOn counts all licenses of the cached type valid on day — the
// license-tally denominator for that day.
func (c *tallyCache) totalValidOn(ctx context.Context, day string) (uint64, error) {
	if err := c.loadLicenses(ctx); err != nil {
		return 0, err
	}
	var n uint64
	for _, l := range c.typeLicenses {
		if licenseValidOn(l, day) {
			n++
		}
	}
	return n, nil
}

func (c *tallyCache) stake(ctx context.Context, addr sdk.AccAddress) (math.Int, error) {
	if s, ok := c.stakes[addr.String()]; ok {
		return s, nil
	}
	s, err := c.k.voterStake(ctx, addr)
	if err != nil {
		return math.ZeroInt(), err
	}
	c.stakes[addr.String()] = s
	return s, nil
}

func (c *tallyCache) totalBondedTokens(ctx context.Context) (math.Int, error) {
	if c.totalBonded != nil {
		return *c.totalBonded, nil
	}
	t, err := c.k.stakingKeeper.TotalBondedTokens(ctx)
	if err != nil {
		return math.ZeroInt(), err
	}
	c.totalBonded = &t
	return t, nil
}

// voterStake returns the bonded stake weight of a voting address. A
// validator-operator address contributes only its self-delegation; any other
// address contributes the sum of its own delegations.
func (k Keeper) voterStake(ctx context.Context, addr sdk.AccAddress) (math.Int, error) {
	valAddr := sdk.ValAddress(addr)

	// If the address operates a validator, count only its self-delegation.
	validator, err := k.stakingKeeper.GetValidator(ctx, valAddr)
	switch {
	case err == nil:
		del, err := k.stakingKeeper.GetDelegation(ctx, addr, valAddr)
		if err != nil {
			if errors.Is(err, stakingtypes.ErrNoDelegation) {
				// Validator with no self-delegation contributes nothing.
				return math.ZeroInt(), nil
			}
			return math.ZeroInt(), err
		}
		return validator.TokensFromShares(del.Shares).TruncateInt(), nil
	case errors.Is(err, stakingtypes.ErrNoValidatorFound):
		// Not a validator operator; fall through to the delegator path.
	default:
		return math.ZeroInt(), err
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
			if errors.Is(err, stakingtypes.ErrNoValidatorFound) {
				// Validator no longer exists; skip this delegation.
				continue
			}
			return math.ZeroInt(), err
		}
		total = total.Add(validator.TokensFromShares(del.Shares).TruncateInt())
	}
	return total, nil
}

// tallyResult is a winning root, the fractions it achieved for each configured
// mechanism, and the header leaf committed in that root.
type tallyResult struct {
	root           []byte
	licenseTally   math.LegacyDec
	stakeTally     math.LegacyDec
	validatorTally math.LegacyDec
	header         *types.DistributionHeader
}

// tallyDistribution groups all votes for a (day, type) by submitted root,
// computes the license-weighted, stake-weighted, and validator-count fractions
// for each, and returns the winning root if any passes every mechanism that the
// type configures (a threshold left empty/"0" is not required). Returns
// (nil, nil) if no root reaches consensus. Iteration is deterministic (sorted by
// root bytes).
//
// Eligibility is judged as of `date`: only votes from signers whose license was
// valid on that day are counted, and the license denominator is the licenses
// valid on that day. The cache memoizes the underlying keeper reads across the
// whole epoch-end pass; the day-specific filter is applied on top.
func (k Keeper) tallyDistribution(ctx context.Context, date, distroType string, dt types.DistributionType, cache *tallyCache) (*tallyResult, error) {
	type group struct {
		licenseWeight   uint64
		stakeWeight     math.Int
		validatorWeight uint64
		header          *types.DistributionHeader
	}
	groups := map[string]*group{}

	rng := collections.NewSuperPrefixedTripleRange[string, string, string](date, distroType)
	err := k.Votes.Walk(ctx, rng, func(key collections.Triple[string, string, string], vote types.DistributionVote) (bool, error) {
		signer := key.K3()

		// A vote only counts while its signer held a license valid on this day.
		lc, err := cache.licensesValidOn(ctx, signer, date)
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
		stake, err := cache.stake(ctx, accAddr)
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
		if cache.isValidator(signer) {
			g.validatorWeight++
		}
		// The header is committed in the root, so every vote for a given root
		// carries the same header; capture the first one seen.
		if g.header == nil {
			g.header = vote.Header
		}
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return nil, nil
	}

	totalLicenses, err := cache.totalValidOn(ctx, date)
	if err != nil {
		return nil, err
	}
	totalBonded, err := cache.totalBondedTokens(ctx)
	if err != nil {
		return nil, err
	}

	// A mechanism is required only when its threshold is configured (non-empty,
	// non-zero) for this type. Params.Validate guarantees at least one is set.
	licenseThreshold, licenseReq, err := optionalThreshold(dt.LicenseTallyThreshold)
	if err != nil {
		return nil, err
	}
	stakeThreshold, stakeReq, err := optionalThreshold(dt.StakeTallyThreshold)
	if err != nil {
		return nil, err
	}
	validatorThreshold, validatorReq, err := optionalThreshold(dt.ValidatorTallyThreshold)
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
		validatorFrac := uintFraction(g.validatorWeight, cache.validatorTotal)

		if licenseReq && !licenseFrac.GTE(licenseThreshold) {
			continue
		}
		if stakeReq && !stakeFrac.GTE(stakeThreshold) {
			continue
		}
		if validatorReq && !validatorFrac.GTE(validatorThreshold) {
			continue
		}
		return &tallyResult{
			root:           []byte(rk),
			licenseTally:   licenseFrac,
			stakeTally:     stakeFrac,
			validatorTally: validatorFrac,
			header:         g.header,
		}, nil
	}
	return nil, nil
}

// optionalThreshold parses a per-type tally threshold. An empty or "0" string
// means the mechanism is not required (required=false); otherwise the parsed
// decimal is returned with required=true.
func optionalThreshold(v string) (math.LegacyDec, bool, error) {
	if v == "" || v == "0" {
		return math.LegacyZeroDec(), false, nil
	}
	dec, err := math.LegacyNewDecFromStr(v)
	if err != nil {
		return math.LegacyZeroDec(), false, err
	}
	return dec, true, nil
}

func uintFraction(num, den uint64) math.LegacyDec {
	if den == 0 {
		return math.LegacyZeroDec()
	}
	// Use NewIntFromUint64 (not int64()) so large counts cannot wrap negative.
	return intFraction(math.NewIntFromUint64(num), math.NewIntFromUint64(den))
}

func intFraction(num, den math.Int) math.LegacyDec {
	if !den.IsPositive() {
		return math.LegacyZeroDec()
	}
	return math.LegacyNewDecFromInt(num).Quo(math.LegacyNewDecFromInt(den))
}
