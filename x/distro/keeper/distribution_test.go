package keeper_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cosmossdk.io/collections"
	"cosmossdk.io/log"
	"cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/testutil"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	epochstypes "github.com/cosmos/cosmos-sdk/x/epochs/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	licensetypes "github.com/webstack-sdk/webstack/x/licenses/types"

	"github.com/TrustedSmartChain/tsc/v3/app"
	"github.com/TrustedSmartChain/tsc/v3/x/distro/keeper"
	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

const (
	testDenom       = "aTSC"
	testLicenseType = "distribution"
	testBond        = "100"
)

// init sets the chain's bech32 prefixes before any address is created, so the
// SDK's address string cache is populated with the correct ("tsc") prefix.
func init() {
	cfg := sdk.GetConfig()
	cfg.SetBech32PrefixForAccount(app.Bech32PrefixAccAddr, app.Bech32PrefixAccPub)
	cfg.SetBech32PrefixForValidator(app.Bech32PrefixValAddr, app.Bech32PrefixValPub)
	cfg.SetBech32PrefixForConsensusNode(app.Bech32PrefixConsAddr, app.Bech32PrefixConsPub)
}

// ---- mock external keepers -------------------------------------------------

type mockAccountKeeper struct{}

func (mockAccountKeeper) GetModuleAddress(string) sdk.AccAddress {
	return authtypes.NewModuleAddress(types.ModuleName)
}
func (mockAccountKeeper) GetModuleAccount(context.Context, string) sdk.ModuleAccountI {
	return nil
}
func (mockAccountKeeper) SetModuleAccount(context.Context, sdk.ModuleAccountI) {}

type mockBankKeeper struct {
	supply   map[string]math.Int
	balances map[string]sdk.Coins
	// blocked simulates x/bank's blocked-address guard: a send to one of these
	// recipients fails, mirroring a blocked module account.
	blocked map[string]bool
}

func newMockBank() *mockBankKeeper {
	return &mockBankKeeper{supply: map[string]math.Int{}, balances: map[string]sdk.Coins{}, blocked: map[string]bool{}}
}
func (m *mockBankKeeper) SpendableCoins(context.Context, sdk.AccAddress) sdk.Coins { return nil }
func (m *mockBankKeeper) MintCoins(_ context.Context, module string, amt sdk.Coins) error {
	for _, c := range amt {
		cur, ok := m.supply[c.Denom]
		if !ok {
			cur = math.ZeroInt()
		}
		m.supply[c.Denom] = cur.Add(c.Amount)
	}
	// Minted coins are credited to the module account, mirroring x/bank.
	m.balances[module] = m.balances[module].Add(amt...)
	return nil
}
func (m *mockBankKeeper) BurnCoins(_ context.Context, module string, amt sdk.Coins) error {
	m.balances[module] = m.balances[module].Sub(amt...)
	return nil
}
func (m *mockBankKeeper) SendCoinsFromModuleToAccount(_ context.Context, module string, recipient sdk.AccAddress, amt sdk.Coins) error {
	if m.blocked[recipient.String()] {
		return fmt.Errorf("%s is not allowed to receive funds", recipient.String())
	}
	m.balances[module] = m.balances[module].Sub(amt...)
	m.balances[recipient.String()] = m.balances[recipient.String()].Add(amt...)
	return nil
}
func (m *mockBankKeeper) SendCoinsFromAccountToModule(_ context.Context, sender sdk.AccAddress, module string, amt sdk.Coins) error {
	m.balances[sender.String()] = m.balances[sender.String()].Sub(amt...)
	m.balances[module] = m.balances[module].Add(amt...)
	return nil
}
func (m *mockBankKeeper) GetSupply(_ context.Context, denom string) sdk.Coin {
	cur, ok := m.supply[denom]
	if !ok {
		cur = math.ZeroInt()
	}
	return sdk.NewCoin(denom, cur)
}

func (m *mockBankKeeper) GetBalance(_ context.Context, addr sdk.AccAddress, denom string) sdk.Coin {
	// This mock keys the distro module account's balance by module name (matching
	// the module string passed to Mint/Send), so translate the module address
	// back to that key; all other accounts are keyed by their bech32 address.
	key := addr.String()
	if addr.Equals(authtypes.NewModuleAddress(types.ModuleName)) {
		key = types.ModuleName
	}
	return sdk.NewCoin(denom, m.balances[key].AmountOf(denom))
}

type mockStakingKeeper struct {
	totalBonded math.Int
	valAddr     sdk.ValAddress
	validator   stakingtypes.Validator
	stake       map[string]math.Int // delegator bech32 -> stake amount
	// optional call counters (nil = not counting) for the memoization test.
	totalBondedCalls *int
	delegationsCalls *int
}

func (m mockStakingKeeper) TotalBondedTokens(context.Context) (math.Int, error) {
	if m.totalBondedCalls != nil {
		*m.totalBondedCalls++
	}
	return m.totalBonded, nil
}
func (m mockStakingKeeper) GetValidator(_ context.Context, addr sdk.ValAddress) (stakingtypes.Validator, error) {
	if addr.Equals(m.valAddr) {
		return m.validator, nil
	}
	return stakingtypes.Validator{}, stakingtypes.ErrNoValidatorFound
}
func (m mockStakingKeeper) GetDelegation(context.Context, sdk.AccAddress, sdk.ValAddress) (stakingtypes.Delegation, error) {
	return stakingtypes.Delegation{}, stakingtypes.ErrNoDelegation
}
func (m mockStakingKeeper) GetDelegatorDelegations(_ context.Context, delegator sdk.AccAddress, _ uint16) ([]stakingtypes.Delegation, error) {
	if m.delegationsCalls != nil {
		*m.delegationsCalls++
	}
	amt, ok := m.stake[delegator.String()]
	if !ok || !amt.IsPositive() {
		return nil, nil
	}
	return []stakingtypes.Delegation{{
		DelegatorAddress: delegator.String(),
		ValidatorAddress: m.valAddr.String(),
		Shares:           math.LegacyNewDecFromInt(amt),
	}}, nil
}

type mockLicensesKeeper struct {
	licenses []licensetypes.License
	// optional call counter (nil = not counting) for the memoization test.
	byTypeCalls *int
}

func (m mockLicensesKeeper) LicensesByHolderAndType(_ context.Context, req *licensetypes.QueryLicensesByHolderAndTypeRequest) (*licensetypes.QueryLicensesByHolderAndTypeResponse, error) {
	var out []licensetypes.License
	for _, l := range m.licenses {
		if l.Holder == req.Holder && l.Type == req.TypeId {
			out = append(out, l)
		}
	}
	return &licensetypes.QueryLicensesByHolderAndTypeResponse{Licenses: out}, nil
}
func (m mockLicensesKeeper) LicensesByType(_ context.Context, req *licensetypes.QueryLicensesByTypeRequest) (*licensetypes.QueryLicensesByTypeResponse, error) {
	if m.byTypeCalls != nil {
		*m.byTypeCalls++
	}
	var out []licensetypes.License
	for _, l := range m.licenses {
		if l.Type == req.TypeId {
			out = append(out, l)
		}
	}
	return &licensetypes.QueryLicensesByTypeResponse{Licenses: out}, nil
}

type mockEpochsKeeper struct{ current int64 }

func (m mockEpochsKeeper) GetEpochInfo(_ sdk.Context, identifier string) (epochstypes.EpochInfo, error) {
	return epochstypes.EpochInfo{Identifier: identifier, CurrentEpoch: m.current}, nil
}

// ---- fixture ---------------------------------------------------------------

type distFixture struct {
	ctx       sdk.Context
	k         keeper.Keeper
	msgServer types.MsgServer
	bank      *mockBankKeeper
}

func setupDistribution(t *testing.T, staking mockStakingKeeper, licenses mockLicensesKeeper, currentEpoch int64) *distFixture {
	t.Helper()

	key := storetypes.NewKVStoreKey(types.StoreKey)
	testCtx := testutil.DefaultContextWithDB(t, key, storetypes.NewTransientStoreKey("transient_"+types.StoreKey))
	ctx := testCtx.Ctx.WithBlockHeight(10)

	encCfg := moduletestutil.MakeTestEncodingConfig()
	bank := newMockBank()

	k := keeper.NewKeeper(
		encCfg.Codec,
		runtime.NewKVStoreService(key),
		log.NewNopLogger(),
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		mockAccountKeeper{},
		bank,
		staking,
		licenses,
		mockEpochsKeeper{current: currentEpoch},
	)

	params := types.DefaultParams()
	params.Denom = testDenom
	params.DistributionLicenseTypeId = testLicenseType
	params.ChallengeBond = testBond // small, test-friendly bond
	params.ReviewDelayDays = 1      // deterministic: one day pending -> live
	require.NoError(t, k.Params.Set(ctx, params))

	return &distFixture{
		ctx:       ctx,
		k:         k,
		msgServer: keeper.NewMsgServerImpl(k),
		bank:      bank,
	}
}

// activeLicense issues a license valid from well before any test day (open
// ended), so it is eligible on every date the tests use.
func activeLicense(holder string) licensetypes.License {
	return licenseFrom(holder, "2000-01-01", "")
}

// licenseFrom issues a license with an explicit [start, end] validity window
// (empty end = open ended), for exercising as-of-day-D eligibility.
func licenseFrom(holder, startDate, endDate string) licensetypes.License {
	return licensetypes.License{Type: testLicenseType, Holder: holder, Status: "active", StartDate: startDate, EndDate: endDate}
}

// ---- tests -----------------------------------------------------------------

func TestSubmitRequiresActiveLicense(t *testing.T) {
	accs := simtestutil.CreateIncrementalAccounts(2)
	signer, valOwner := accs[0], accs[1]
	f := setupDistribution(t,
		mockStakingKeeper{totalBonded: math.NewInt(100), valAddr: sdk.ValAddress(valOwner), validator: unitValidator(sdk.ValAddress(valOwner))},
		mockLicensesKeeper{}, // no licenses issued
		5,
	)

	_, err := f.msgServer.SubmitDistributionRoot(f.ctx, &types.MsgSubmitDistributionRoot{
		Signer:     signer.String(),
		Date:       dateOf(1),
		DistroType: defaultType,
		MerkleRoot: []byte("root"), TotalsByCategory: defaultHeaderTotals(),
	})
	require.ErrorContains(t, err, "no license")
}

func TestSubmitRejectsFutureEpoch(t *testing.T) {
	accs := simtestutil.CreateIncrementalAccounts(2)
	signer, valOwner := accs[0], accs[1]
	f := setupDistribution(t,
		mockStakingKeeper{totalBonded: math.NewInt(100), valAddr: sdk.ValAddress(valOwner), validator: unitValidator(sdk.ValAddress(valOwner))},
		mockLicensesKeeper{licenses: []licensetypes.License{activeLicense(signer.String())}},
		3, // current epoch
	)

	_, err := f.msgServer.SubmitDistributionRoot(f.ctx, &types.MsgSubmitDistributionRoot{
		Signer:     signer.String(),
		Date:       dateOf(9), // future
		DistroType: defaultType,
		MerkleRoot: []byte("root"), TotalsByCategory: defaultHeaderTotals(),
	})
	require.ErrorContains(t, err, "future")
}

func TestDistributionFlow(t *testing.T) {
	// Four licensed stakers; three vote root A, one votes root B.
	accs := simtestutil.CreateIncrementalAccounts(6)
	voters := accs[0:4]
	recipient := accs[4]
	valOwner := accs[5]

	// Build the merkle tree the voting nodes would have produced for root A: a
	// header leaf plus two reward leaves.
	rewards := []rewardLeaf{
		{nonce: 0, addr: recipient.String(), total: "1000", cats: map[string]string{"type1": "1000"}},
		{nonce: 1, addr: voters[0].String(), total: "2000", cats: map[string]string{"type1": "2000"}},
	}
	rootA, hproofA, rewardProofs := buildSubmission(defaultType, dateOf(1), 0, defaultHeaderTotals(), rewards)
	rootB, hproofB := headerOnlySubmission(dateOf(1), 1) // distinct, losing root

	stake := map[string]math.Int{
		voters[0].String(): math.NewInt(40),
		voters[1].String(): math.NewInt(20),
		voters[2].String(): math.NewInt(10),
		voters[3].String(): math.NewInt(30),
	}
	valAddr := sdk.ValAddress(valOwner)
	staking := mockStakingKeeper{
		totalBonded: math.NewInt(100),
		valAddr:     valAddr,
		validator:   unitValidator(valAddr),
		stake:       stake,
	}
	licenses := mockLicensesKeeper{licenses: []licensetypes.License{
		activeLicense(voters[0].String()),
		activeLicense(voters[1].String()),
		activeLicense(voters[2].String()),
		activeLicense(voters[3].String()),
	}}

	f := setupDistribution(t, staking, licenses, 1)

	const epoch = int64(1)
	// voters 0,1,2 vote root A (license 3/4 = 0.75, stake 70/100 = 0.70).
	for _, v := range []sdk.AccAddress{voters[0], voters[1], voters[2]} {
		f.submitRoot(t, v, epoch, rootA, hproofA)
	}
	// voter 3 votes root B (version 1).
	f.submitRootVersion(t, voters[3], epoch, 1, rootB, hproofB)

	// Before finalization, claims are rejected.
	_, err := f.msgServer.Claim(f.ctx, &types.MsgClaim{
		Claimer: recipient.String(), Date: dateOf(epoch), DistroType: defaultType, Nonce: 0, Address: recipient.String(), Total: "1000", Categories: map[string]string{"type1": "1000"}, Proof: rewardProofs[0],
	})
	require.ErrorContains(t, err, "not live")

	// First epoch end: root A wins and the epoch enters PENDING (review delay).
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", epoch))
	ed, err := f.k.Distributions.Get(f.ctx, collections.Join(dateOf(epoch), defaultType))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_PENDING, ed.Status)
	require.Equal(t, rootA, ed.MerkleRoot)

	// Claims are still rejected while pending.
	_, err = f.msgServer.Claim(f.ctx, &types.MsgClaim{
		Claimer: recipient.String(), Date: dateOf(epoch), DistroType: defaultType, Nonce: 0, Address: recipient.String(), Total: "1000", Categories: map[string]string{"type1": "1000"}, Proof: rewardProofs[0],
	})
	require.ErrorContains(t, err, "not live")

	// Next epoch end: the review delay (1) has elapsed, so it auto-promotes to LIVE.
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", epoch+1))
	ed, err = f.k.Distributions.Get(f.ctx, collections.Join(dateOf(epoch), defaultType))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_LIVE, ed.Status)
	require.Equal(t, rootA, ed.MerkleRoot)

	// Claim reward nonce 0 for the recipient.
	_, err = f.msgServer.Claim(f.ctx, &types.MsgClaim{
		Claimer: recipient.String(), Date: dateOf(epoch), DistroType: defaultType, Nonce: 0, Address: recipient.String(), Total: "1000", Categories: map[string]string{"type1": "1000"}, Proof: rewardProofs[0],
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(1000), f.bank.balances[recipient.String()].AmountOf(testDenom))

	// Double claim of the same nonce is rejected.
	_, err = f.msgServer.Claim(f.ctx, &types.MsgClaim{
		Claimer: recipient.String(), Date: dateOf(epoch), DistroType: defaultType, Nonce: 0, Address: recipient.String(), Total: "1000", Categories: map[string]string{"type1": "1000"}, Proof: rewardProofs[0],
	})
	require.ErrorContains(t, err, "already claimed")

	// A claim with a tampered amount (for the still-unclaimed nonce 1) fails
	// proof verification.
	_, err = f.msgServer.Claim(f.ctx, &types.MsgClaim{
		Claimer: voters[0].String(), Date: dateOf(epoch), DistroType: defaultType, Nonce: 1, Address: voters[0].String(), Total: "9999", Categories: map[string]string{"type1": "9999"}, Proof: rewardProofs[1],
	})
	require.ErrorContains(t, err, "invalid merkle proof")
}

func TestTallyBelowThresholdStaysVoting(t *testing.T) {
	accs := simtestutil.CreateIncrementalAccounts(5)
	voters := accs[0:4]
	valOwner := accs[4]

	// Only two of four licensed voters vote => license 2/4 = 0.5 < 0.667.
	stake := map[string]math.Int{
		voters[0].String(): math.NewInt(40),
		voters[1].String(): math.NewInt(40),
	}
	valAddr := sdk.ValAddress(valOwner)
	staking := mockStakingKeeper{totalBonded: math.NewInt(100), valAddr: valAddr, validator: unitValidator(valAddr), stake: stake}
	licenses := mockLicensesKeeper{licenses: []licensetypes.License{
		activeLicense(voters[0].String()), activeLicense(voters[1].String()),
		activeLicense(voters[2].String()), activeLicense(voters[3].String()),
	}}

	f := setupDistribution(t, staking, licenses, 1)
	for _, v := range []sdk.AccAddress{voters[0], voters[1]} {
		f.submit(t, v, 1, 0)
	}

	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 1))

	ed, err := f.k.Distributions.Get(f.ctx, collections.Join(dateOf(1), defaultType))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_VOTING, ed.Status)
}

func TestClaimRespectsEpochBudget(t *testing.T) {
	f, voters := fourVoterConsensus(t)
	recipient := simtestutil.CreateIncrementalAccounts(1)[0]

	// nonce 0 claims more than a single day's halving allocation; nonce 1 is small.
	const epoch = int64(1)
	overBudget := "1000000000000000000000000" // 1e24, far above one day's budget but below MaxSupply
	rewards := []rewardLeaf{
		{nonce: 0, addr: recipient.String(), total: overBudget, cats: map[string]string{"type1": overBudget}},
		{nonce: 1, addr: recipient.String(), total: "1000", cats: map[string]string{"type1": "1000"}},
	}
	root, hproof, rewardProofs := buildSubmission(defaultType, dateOf(epoch), 0, defaultHeaderTotals(), rewards)

	for _, v := range voters {
		f.submitRoot(t, v, epoch, root, hproof)
	}
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", epoch))   // -> PENDING
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", epoch+1)) // -> LIVE

	// A within-budget claim succeeds.
	_, err := f.msgServer.Claim(f.ctx, &types.MsgClaim{
		Claimer: recipient.String(), Date: dateOf(epoch), DistroType: defaultType, Nonce: 1, Address: recipient.String(), Total: "1000", Categories: map[string]string{"type1": "1000"}, Proof: rewardProofs[1],
	})
	require.NoError(t, err)

	// A claim exceeding the epoch's halving budget is rejected (before max supply).
	_, err = f.msgServer.Claim(f.ctx, &types.MsgClaim{
		Claimer: recipient.String(), Date: dateOf(epoch), DistroType: defaultType, Nonce: 0, Address: recipient.String(), Total: overBudget, Categories: map[string]string{"type1": overBudget}, Proof: rewardProofs[0],
	})
	require.ErrorContains(t, err, "budget")

	// The cumulative claimed amount reflects only the successful claim.
	ed, err := f.k.Distributions.Get(f.ctx, collections.Join(dateOf(epoch), defaultType))
	require.NoError(t, err)
	require.Equal(t, "1000", ed.ClaimedAmount)
}

func TestClaimCategoryTotals(t *testing.T) {
	f, voters := fourVoterConsensus(t)
	recipient := simtestutil.CreateIncrementalAccounts(1)[0]

	const epoch = int64(1)
	// nonce 0 splits across two categories; nonce 1 adds to one of them.
	cats0 := map[string]string{"type1": "1000", "type2": "2000"}
	cats1 := map[string]string{"type1": "1500"}
	rewards := []rewardLeaf{
		{nonce: 0, addr: recipient.String(), total: "3000", cats: cats0},
		{nonce: 1, addr: recipient.String(), total: "1500", cats: cats1},
	}
	root, hproof, rewardProofs := buildSubmission(defaultType, dateOf(epoch), 0, defaultHeaderTotals(), rewards)

	for _, v := range voters {
		f.submitRoot(t, v, epoch, root, hproof)
	}
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", epoch))   // -> PENDING
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", epoch+1)) // -> LIVE

	_, err := f.msgServer.Claim(f.ctx, &types.MsgClaim{
		Claimer: recipient.String(), Date: dateOf(epoch), DistroType: defaultType, Nonce: 0, Address: recipient.String(),
		Total: "3000", Categories: cats0, Proof: rewardProofs[0],
	})
	require.NoError(t, err)
	_, err = f.msgServer.Claim(f.ctx, &types.MsgClaim{
		Claimer: recipient.String(), Date: dateOf(epoch), DistroType: defaultType, Nonce: 1, Address: recipient.String(),
		Total: "1500", Categories: cats1, Proof: rewardProofs[1],
	})
	require.NoError(t, err)

	querier := keeper.NewQuerier(f.k)

	// type1 accumulates across both claims, type2 from the first only.
	byDate, err := querier.ClaimTotalByCategory(f.ctx, &types.QueryClaimTotalByCategoryRequest{Date: dateOf(epoch), DistroType: defaultType})
	require.NoError(t, err)
	require.Equal(t, dateOf(epoch), byDate.Date)
	require.Equal(t, map[string]string{"type1": "2500", "type2": "2000"}, byDate.Totals)

	// dateOf(1) is exactly the default start date.
	require.Equal(t, types.DefaultDistributionStartDate, dateOf(epoch))

	// Both nonces were claimed; ClaimsByDate lists them in ascending order.
	claims, err := querier.ClaimsByDate(f.ctx, &types.QueryClaimsByDateRequest{Date: dateOf(epoch), DistroType: defaultType})
	require.NoError(t, err)
	require.Equal(t, []uint64{0, 1}, claims.Nonces)

	// A day with no claims returns an empty list.
	empty, err := querier.ClaimsByDate(f.ctx, &types.QueryClaimsByDateRequest{Date: dateOf(epoch + 1), DistroType: defaultType})
	require.NoError(t, err)
	require.Empty(t, empty.Nonces)
}

func TestClaimRejectsInconsistentCategories(t *testing.T) {
	f, voters := fourVoterConsensus(t)
	recipient := simtestutil.CreateIncrementalAccounts(1)[0]

	const epoch = int64(1)
	cats := map[string]string{"type1": "1000", "type2": "2000"}
	// A second reward leaf so the tree (and proof) is non-trivial.
	rewards := []rewardLeaf{
		{nonce: 0, addr: recipient.String(), total: "3000", cats: cats},
		{nonce: 1, addr: recipient.String(), total: "5", cats: map[string]string{"type1": "5"}},
	}
	root, hproof, rewardProofs := buildSubmission(defaultType, dateOf(epoch), 0, defaultHeaderTotals(), rewards)

	for _, v := range voters {
		f.submitRoot(t, v, epoch, root, hproof)
	}
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", epoch))
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", epoch+1))

	// total != sum(categories) is rejected before proof verification.
	_, err := f.msgServer.Claim(f.ctx, &types.MsgClaim{
		Claimer: recipient.String(), Date: dateOf(epoch), DistroType: defaultType, Nonce: 0, Address: recipient.String(),
		Total: "9999", Categories: cats, Proof: rewardProofs[0],
	})
	require.ErrorContains(t, err, "does not equal total")

	// A tampered breakdown (correct total) fails the merkle proof.
	_, err = f.msgServer.Claim(f.ctx, &types.MsgClaim{
		Claimer: recipient.String(), Date: dateOf(epoch), DistroType: defaultType, Nonce: 0, Address: recipient.String(),
		Total: "3000", Categories: map[string]string{"type1": "2000", "type2": "1000"}, Proof: rewardProofs[0],
	})
	require.ErrorContains(t, err, "invalid merkle proof")

	// Empty categories are rejected.
	_, err = f.msgServer.Claim(f.ctx, &types.MsgClaim{
		Claimer: recipient.String(), Date: dateOf(epoch), DistroType: defaultType, Nonce: 0, Address: recipient.String(),
		Total: "3000", Categories: nil, Proof: rewardProofs[0],
	})
	require.ErrorContains(t, err, "categories must not be empty")
}

// TestClaimRejectsUnconfiguredCategory checks a claim whose reward breakdown
// names a category not configured on the distribution type is rejected, even
// though the leaf is a valid member of the finalized tree. Category names are
// always validated against the type config, independent of category caps.
func TestClaimRejectsUnconfiguredCategory(t *testing.T) {
	f, voters := fourVoterConsensus(t)
	recipient := simtestutil.CreateIncrementalAccounts(1)[0]

	const epoch = int64(1)
	// "type3" is not one of the default type's configured categories
	// ("type1"/"type2"), but LeafHash does not check config, so the tree is valid.
	rewards := []rewardLeaf{
		{nonce: 0, addr: recipient.String(), total: "1000", cats: map[string]string{"type3": "1000"}},
	}
	root, hproof, rewardProofs := buildSubmission(defaultType, dateOf(epoch), 0, defaultHeaderTotals(), rewards)

	for _, v := range voters {
		f.submitRoot(t, v, epoch, root, hproof)
	}
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", epoch))
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", epoch+1))

	_, err := f.msgServer.Claim(f.ctx, &types.MsgClaim{
		Claimer: recipient.String(), Date: dateOf(epoch), DistroType: defaultType, Nonce: 0, Address: recipient.String(),
		Total: "1000", Categories: map[string]string{"type3": "1000"}, Proof: rewardProofs[0],
	})
	require.ErrorContains(t, err, "is not configured for distribution type")
}

// TestClaimFourLeafProof exercises a multi-level (2-element) merkle proof
// end-to-end through the claim handler: a 4-leaf tree is voted to consensus,
// promoted to LIVE, then a leaf is claimed with its sibling and opposite-subtree
// hash. The 2-leaf trees the other tests use only fold a single sibling, so this
// is the only on-chain coverage of proof depth > 1.
func TestClaimFourLeafProof(t *testing.T) {
	f, voters := fourVoterConsensus(t)
	accs := simtestutil.CreateIncrementalAccounts(4)

	const epoch = int64(1)
	// Build a tree with a header leaf plus four reward leaves, yielding multi-level
	// (depth > 1) inclusion proofs for the reward leaves.
	rewards := []rewardLeaf{
		{nonce: 0, addr: accs[0].String(), total: "10", cats: map[string]string{"type1": "10"}},
		{nonce: 1, addr: accs[1].String(), total: "20", cats: map[string]string{"type1": "20"}},
		{nonce: 2, addr: accs[2].String(), total: "30", cats: map[string]string{"type1": "30"}},
		{nonce: 3, addr: accs[3].String(), total: "40", cats: map[string]string{"type1": "40"}},
	}
	root, hproof, rewardProofs := buildSubmission(defaultType, dateOf(epoch), 0, defaultHeaderTotals(), rewards)
	require.Greater(t, len(rewardProofs[2]), 1, "this test must exercise a multi-element proof")

	for _, v := range voters {
		f.submitRoot(t, v, epoch, root, hproof)
	}
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", epoch))   // -> PENDING
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", epoch+1)) // -> LIVE

	// Claim reward 2 with its multi-element inclusion proof.
	_, err := f.msgServer.Claim(f.ctx, &types.MsgClaim{
		Claimer: accs[2].String(), Date: dateOf(epoch), DistroType: defaultType, Nonce: 2, Address: accs[2].String(),
		Total: "30", Categories: map[string]string{"type1": "30"}, Proof: rewardProofs[2],
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(30), f.bank.balances[accs[2].String()].AmountOf(testDenom))

	// And reward 0.
	_, err = f.msgServer.Claim(f.ctx, &types.MsgClaim{
		Claimer: accs[0].String(), Date: dateOf(epoch), DistroType: defaultType, Nonce: 0, Address: accs[0].String(),
		Total: "10", Categories: map[string]string{"type1": "10"}, Proof: rewardProofs[0],
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(10), f.bank.balances[accs[0].String()].AmountOf(testDenom))

	// A correct leaf with a reversed proof (wrong fold order) fails verification.
	reversed := make([][]byte, len(rewardProofs[1]))
	for i, h := range rewardProofs[1] {
		reversed[len(reversed)-1-i] = h
	}
	_, err = f.msgServer.Claim(f.ctx, &types.MsgClaim{
		Claimer: accs[1].String(), Date: dateOf(epoch), DistroType: defaultType, Nonce: 1, Address: accs[1].String(),
		Total: "20", Categories: map[string]string{"type1": "20"}, Proof: reversed,
	})
	require.ErrorContains(t, err, "invalid merkle proof")
}

func TestSubmitRejectsDateBeforeStart(t *testing.T) {
	accs := simtestutil.CreateIncrementalAccounts(2)
	signer, valOwner := accs[0], accs[1]
	f := setupDistribution(t,
		mockStakingKeeper{totalBonded: math.NewInt(100), valAddr: sdk.ValAddress(valOwner), validator: unitValidator(sdk.ValAddress(valOwner))},
		mockLicensesKeeper{licenses: []licensetypes.License{activeLicense(signer.String())}},
		3,
	)

	// The day before the distribution start date is not a valid distribution day.
	_, err := f.msgServer.SubmitDistributionRoot(f.ctx, &types.MsgSubmitDistributionRoot{
		Signer:     signer.String(),
		Date:       "2025-07-21", // start date is 2025-07-22 (dateOf(1))
		DistroType: defaultType,
		MerkleRoot: []byte("root"), TotalsByCategory: defaultHeaderTotals(),
	})
	require.ErrorContains(t, err, "before the distribution start date")
}

func TestSubmitRejectsDateOlderThanWindow(t *testing.T) {
	accs := simtestutil.CreateIncrementalAccounts(2)
	signer, valOwner := accs[0], accs[1]
	// current day is epoch 10; the default voting window is 7 days, so day 1 is
	// 9 days old and can no longer reach consensus.
	f := setupDistribution(t,
		mockStakingKeeper{totalBonded: math.NewInt(100), valAddr: sdk.ValAddress(valOwner), validator: unitValidator(sdk.ValAddress(valOwner))},
		mockLicensesKeeper{licenses: []licensetypes.License{activeLicense(signer.String())}},
		10,
	)

	root, hproof := headerOnlySubmission(dateOf(1), 0)
	_, err := f.msgServer.SubmitDistributionRoot(f.ctx, &types.MsgSubmitDistributionRoot{
		Signer:     signer.String(),
		Date:       dateOf(1),
		DistroType: defaultType,
		MerkleRoot: root, TotalsByCategory: defaultHeaderTotals(), HeaderProof: hproof,
	})
	require.ErrorContains(t, err, "older than the voting window")
}

// TestCategoryTotalsIsolatedByDate proves the per-category running totals (and
// the ClaimTotalByCategory query) are scoped to a single day: claims on two
// different LIVE days do not bleed into each other.
func TestCategoryTotalsIsolatedByDate(t *testing.T) {
	accs := simtestutil.CreateIncrementalAccounts(6)
	voters := accs[0:4]
	recipient := accs[4]
	valOwner := accs[5]
	stake := map[string]math.Int{
		voters[0].String(): math.NewInt(25), voters[1].String(): math.NewInt(25),
		voters[2].String(): math.NewInt(25), voters[3].String(): math.NewInt(25),
	}
	valAddr := sdk.ValAddress(valOwner)
	staking := mockStakingKeeper{totalBonded: math.NewInt(100), valAddr: valAddr, validator: unitValidator(valAddr), stake: stake}
	licenses := mockLicensesKeeper{licenses: []licensetypes.License{
		activeLicense(voters[0].String()), activeLicense(voters[1].String()),
		activeLicense(voters[2].String()), activeLicense(voters[3].String()),
	}}
	f := setupDistribution(t, staking, licenses, 3) // current day = 3, so days 1 and 2 are open

	// Each day has a single reward leaf (alongside the header leaf).
	day1Rewards := []rewardLeaf{{nonce: 0, addr: recipient.String(), total: "1000", cats: map[string]string{"type1": "1000"}}}
	day2Rewards := []rewardLeaf{{nonce: 0, addr: recipient.String(), total: "2000", cats: map[string]string{"type1": "2000"}}}
	root1, hproof1, rewardProofs1 := buildSubmission(defaultType, dateOf(1), 0, defaultHeaderTotals(), day1Rewards)
	root2, hproof2, rewardProofs2 := buildSubmission(defaultType, dateOf(2), 0, defaultHeaderTotals(), day2Rewards)
	for _, v := range voters {
		f.submitRoot(t, v, 1, root1, hproof1)
		f.submitRoot(t, v, 2, root2, hproof2)
	}
	// Advance: day1 -> LIVE by epoch 2, day2 -> LIVE by epoch 3 (review delay 1).
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 1))
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 2))
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 3))

	_, err := f.msgServer.Claim(f.ctx, &types.MsgClaim{
		Claimer: recipient.String(), Date: dateOf(1), DistroType: defaultType, Nonce: 0, Address: recipient.String(),
		Total: "1000", Categories: map[string]string{"type1": "1000"}, Proof: rewardProofs1[0],
	})
	require.NoError(t, err)
	_, err = f.msgServer.Claim(f.ctx, &types.MsgClaim{
		Claimer: recipient.String(), Date: dateOf(2), DistroType: defaultType, Nonce: 0, Address: recipient.String(),
		Total: "2000", Categories: map[string]string{"type1": "2000"}, Proof: rewardProofs2[0],
	})
	require.NoError(t, err)

	querier := keeper.NewQuerier(f.k)
	day1, err := querier.ClaimTotalByCategory(f.ctx, &types.QueryClaimTotalByCategoryRequest{Date: dateOf(1), DistroType: defaultType})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"type1": "1000"}, day1.Totals)

	day2, err := querier.ClaimTotalByCategory(f.ctx, &types.QueryClaimTotalByCategoryRequest{Date: dateOf(2), DistroType: defaultType})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"type1": "2000"}, day2.Totals)
}

// TestClaimRespectsMaxSupply exercises the final supply bound: a claim that is
// within the day's halving budget is still rejected if minting it would push
// total supply over max_supply.
func TestClaimRespectsMaxSupply(t *testing.T) {
	f, voters := fourVoterConsensus(t)
	recipient := simtestutil.CreateIncrementalAccounts(1)[0]

	const epoch = int64(1)
	amount := "100000000000000000000" // 1e20, comfortably within day 1's budget
	rewards := []rewardLeaf{{nonce: 0, addr: recipient.String(), total: amount, cats: map[string]string{"type1": amount}}}
	root, hproof, rewardProofs := buildSubmission(defaultType, dateOf(epoch), 0, defaultHeaderTotals(), rewards)
	for _, v := range voters {
		f.submitRoot(t, v, epoch, root, hproof)
	}
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", epoch))   // -> PENDING
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", epoch+1)) // -> LIVE

	// Pre-seed circulating supply at the cap so any mint, even within the day's
	// budget, pushes total supply over max_supply.
	maxSupply, ok := math.NewIntFromString(types.DefaultMaxSupply)
	require.True(t, ok)
	f.bank.supply[testDenom] = maxSupply

	_, err := f.msgServer.Claim(f.ctx, &types.MsgClaim{
		Claimer: recipient.String(), Date: dateOf(epoch), DistroType: defaultType, Nonce: 0, Address: recipient.String(),
		Total: amount, Categories: map[string]string{"type1": amount}, Proof: rewardProofs[0],
	})
	require.ErrorContains(t, err, "max supply exceeded")
}

func TestVotingExpiresAfterWindow(t *testing.T) {
	accs := simtestutil.CreateIncrementalAccounts(2)
	signer, other := accs[0], accs[1]
	// Two licensed holders but only one votes => never reaches consensus.
	f := setupDistribution(t,
		mockStakingKeeper{totalBonded: math.NewInt(100), valAddr: sdk.ValAddress(other), validator: unitValidator(sdk.ValAddress(other)), stake: map[string]math.Int{signer.String(): math.NewInt(10)}},
		mockLicensesKeeper{licenses: []licensetypes.License{activeLicense(signer.String()), activeLicense(other.String())}},
		1,
	)
	f.submit(t, signer, 1, 0)

	// Within the voting window it stays VOTING.
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 1))
	ed, err := f.k.Distributions.Get(f.ctx, collections.Join(dateOf(1), defaultType))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_VOTING, ed.Status)

	// Once the window (default 7) has elapsed it expires and is no longer tallied.
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 1+int64(types.DefaultVoteWindowDays)))
	ed, err = f.k.Distributions.Get(f.ctx, collections.Join(dateOf(1), defaultType))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_EXPIRED, ed.Status)
}

func TestChallengeDoesNotResetReviewDelay(t *testing.T) {
	f, voters := fourVoterConsensus(t)
	params, err := f.k.Params.Get(f.ctx)
	require.NoError(t, err)
	params.ReviewDelayDays = 2 // longer delay so a reset would be observable
	require.NoError(t, f.k.Params.Set(f.ctx, params))

	const epoch = int64(1)
	for _, v := range voters {
		f.submit(t, v, epoch, 0)
	}

	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 1)) // VOTING -> PENDING (since=1)
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 2)) // still PENDING (2-1 < 2)

	// Challenge during the window; the retained votes re-confirm the same root.
	f.bank.balances[voters[0].String()] = sdk.NewCoins(sdk.NewCoin(testDenom, math.NewInt(100)))
	_, err = f.msgServer.ChallengeDistribution(f.ctx, &types.MsgChallengeDistribution{Challenger: voters[0].String(), Date: dateOf(epoch), DistroType: defaultType})
	require.NoError(t, err)

	// Re-tally resolves back to PENDING but must NOT restart the review timer.
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 3))
	ed, err := f.k.Distributions.Get(f.ctx, collections.Join(dateOf(epoch), defaultType))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_PENDING, ed.Status)
	require.Equal(t, dateOf(1), ed.PendingSinceDate, "review timer must resume, not reset")

	// With the original timer (since=1, delay=2) it promotes at epoch 4; a reset
	// (since=3) would leave it PENDING here.
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 4))
	ed, err = f.k.Distributions.Get(f.ctx, collections.Join(dateOf(epoch), defaultType))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_LIVE, ed.Status)
}

// fourVoterConsensus sets up four equally-staked, licensed voters (25 stake
// each of 100 bonded) and returns the fixture and the voters.
func fourVoterConsensus(t *testing.T) (*distFixture, []sdk.AccAddress) {
	t.Helper()
	accs := simtestutil.CreateIncrementalAccounts(5)
	voters := accs[0:4]
	valOwner := accs[4]
	stake := map[string]math.Int{
		voters[0].String(): math.NewInt(25),
		voters[1].String(): math.NewInt(25),
		voters[2].String(): math.NewInt(25),
		voters[3].String(): math.NewInt(25),
	}
	valAddr := sdk.ValAddress(valOwner)
	staking := mockStakingKeeper{totalBonded: math.NewInt(100), valAddr: valAddr, validator: unitValidator(valAddr), stake: stake}
	licenses := mockLicensesKeeper{licenses: []licensetypes.License{
		activeLicense(voters[0].String()), activeLicense(voters[1].String()),
		activeLicense(voters[2].String()), activeLicense(voters[3].String()),
	}}
	return setupDistribution(t, staking, licenses, 1), voters
}

// submit votes a header-only consensus root for the given epoch. The version
// distinguishes otherwise-identical roots (e.g. competing roots in a
// no-consensus test): callers that want a distinct root pass a distinct
// version. Use submitRoot for claim tests that need a real reward tree.
func (f *distFixture) submit(t *testing.T, signer sdk.AccAddress, epoch int64, version uint64) {
	t.Helper()
	root, hproof := headerOnlySubmission(dateOf(epoch), version)
	f.submitRootVersion(t, signer, epoch, version, root, hproof)
}

// submitRoot votes a pre-built (version 0) root with its header inclusion proof.
// Used by claim tests that build a reward tree with buildSubmission, which always
// uses version 0.
func (f *distFixture) submitRoot(t *testing.T, signer sdk.AccAddress, epoch int64, root []byte, headerProof [][]byte) {
	t.Helper()
	f.submitRootVersion(t, signer, epoch, 0, root, headerProof)
}

// submitRootVersion votes a pre-built root whose header leaf was built at the
// given version; the submitted Version must match the header for inclusion.
func (f *distFixture) submitRootVersion(t *testing.T, signer sdk.AccAddress, epoch int64, version uint64, root []byte, headerProof [][]byte) {
	t.Helper()
	_, err := f.msgServer.SubmitDistributionRoot(f.ctx, &types.MsgSubmitDistributionRoot{
		Signer: signer.String(), Date: dateOf(epoch), DistroType: defaultType,
		MerkleRoot: root, Version: version, TotalsByCategory: defaultHeaderTotals(), HeaderProof: headerProof,
	})
	require.NoError(t, err)
}

// dateOf maps an epoch number to its distribution day (YYYY-MM-DD) using the
// default start date, mirroring how the keeper translates the x/epochs hook.
// Epoch 1 is the start date; epoch N is startDate+(N-1) days.
func dateOf(epoch int64) string {
	start, err := time.Parse("2006-01-02", types.DefaultDistributionStartDate)
	if err != nil {
		panic(err)
	}
	return start.AddDate(0, 0, int(epoch-1)).Format("2006-01-02")
}

func TestChallengeFrivolousBurnsBond(t *testing.T) {
	f, voters := fourVoterConsensus(t)
	rootA, _ := headerOnlySubmission(dateOf(1), 0)

	for _, v := range voters {
		f.submit(t, v, 1, 0)
	}
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 1)) // -> PENDING

	// Fund and challenge.
	f.bank.balances[voters[0].String()] = sdk.NewCoins(sdk.NewCoin(testDenom, math.NewInt(100)))
	_, err := f.msgServer.ChallengeDistribution(f.ctx, &types.MsgChallengeDistribution{Challenger: voters[0].String(), Date: dateOf(1), DistroType: defaultType})
	require.NoError(t, err)

	ed, err := f.k.Distributions.Get(f.ctx, collections.Join(dateOf(1), defaultType))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_UNDER_REVIEW, ed.Status)
	require.Equal(t, math.NewInt(100), f.bank.balances[types.ModuleName].AmountOf(testDenom)) // escrowed
	require.True(t, f.bank.balances[voters[0].String()].AmountOf(testDenom).IsZero())

	// No re-vote: retained votes still produce root A => frivolous => bond burned.
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 2))
	ed, err = f.k.Distributions.Get(f.ctx, collections.Join(dateOf(1), defaultType))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_PENDING, ed.Status)
	require.Equal(t, rootA, ed.MerkleRoot)
	require.True(t, f.bank.balances[voters[0].String()].AmountOf(testDenom).IsZero()) // not refunded
	require.True(t, f.bank.balances[types.ModuleName].AmountOf(testDenom).IsZero())   // burned
}

func TestChallengeUpheldRefundsBondAndAdoptsNewRoot(t *testing.T) {
	f, voters := fourVoterConsensus(t)
	rootB, _ := headerOnlySubmission(dateOf(1), 1) // corrected root (version 1)

	for _, v := range voters {
		f.submit(t, v, 1, 0) // original (version 0)
	}
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 1)) // -> PENDING (rootA)

	f.bank.balances[voters[0].String()] = sdk.NewCoins(sdk.NewCoin(testDenom, math.NewInt(100)))
	_, err := f.msgServer.ChallengeDistribution(f.ctx, &types.MsgChallengeDistribution{Challenger: voters[0].String(), Date: dateOf(1), DistroType: defaultType})
	require.NoError(t, err)

	// Re-vote: a supermajority now submits the corrected root B.
	f.submit(t, voters[0], 1, 1)
	f.submit(t, voters[1], 1, 1)
	f.submit(t, voters[2], 1, 1)

	// Re-tally adopts root B (!= challenged root) => challenge upheld => bond refunded.
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 2))
	ed, err := f.k.Distributions.Get(f.ctx, collections.Join(dateOf(1), defaultType))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_PENDING, ed.Status)
	require.Equal(t, rootB, ed.MerkleRoot)
	require.Equal(t, math.NewInt(100), f.bank.balances[voters[0].String()].AmountOf(testDenom)) // refunded
	require.True(t, f.bank.balances[types.ModuleName].AmountOf(testDenom).IsZero())

	// And after the delay it promotes to LIVE with the corrected root.
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 3))
	ed, err = f.k.Distributions.Get(f.ctx, collections.Join(dateOf(1), defaultType))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_LIVE, ed.Status)
	require.Equal(t, rootB, ed.MerkleRoot)
}

// unitValidator builds a validator whose TokensFromShares is the identity, so a
// delegation with Shares == amount yields exactly `amount` tokens.
func unitValidator(valAddr sdk.ValAddress) stakingtypes.Validator {
	return stakingtypes.Validator{
		OperatorAddress: valAddr.String(),
		Tokens:          math.OneInt(),
		DelegatorShares: math.LegacyOneDec(),
	}
}
