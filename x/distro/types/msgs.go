package types

import (
	"fmt"
	"time"

	"cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	_ sdk.Msg = &MsgUpdateParams{}
	_ sdk.Msg = &MsgSubmitDistributionRoot{}
	_ sdk.Msg = &MsgClaim{}
	_ sdk.Msg = &MsgChallengeDistribution{}
	_ sdk.Msg = &MsgReviveDistribution{}
)

// validateDateString checks a date is a non-empty YYYY-MM-DD string.
func validateDateString(date string) error {
	if date == "" {
		return fmt.Errorf("date cannot be empty")
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return fmt.Errorf("date %q must be in YYYY-MM-DD format: %w", date, err)
	}
	return nil
}

// NewMsgUpdateParams creates new instance of MsgUpdateParams
func NewMsgUpdateParams(
	sender sdk.Address,
	mintingAddress string,
	receivingAddress string,
	denom string,
	maxSupply string,
	distroStartDate string,
	monthsInHalvingPeriod uint64,
	distributionLicenseTypeID string,
	licenseTallyThreshold string,
	stakeTallyThreshold string,
	epochIdentifier string,
	reviewDelayDays uint64,
	challengeBond string,
	voteWindowDays uint64,
) *MsgUpdateParams {
	return &MsgUpdateParams{
		Authority: sender.String(),
		Params: NewParams(
			mintingAddress,
			receivingAddress,
			denom,
			maxSupply,
			distroStartDate,
			monthsInHalvingPeriod,
			distributionLicenseTypeID,
			licenseTallyThreshold,
			stakeTallyThreshold,
			epochIdentifier,
			reviewDelayDays,
			challengeBond,
			voteWindowDays,
		),
	}
}

// Route returns the name of the module
func (msg MsgUpdateParams) Route() string { return ModuleName }

// Type returns the the action
func (msg MsgUpdateParams) Type() string { return "update_params" }

// GetSignBytes implements the LegacyMsg interface.
func (msg MsgUpdateParams) GetSignBytes() []byte {
	return sdk.MustSortJSON(AminoCdc.MustMarshalJSON(&msg))
}

// GetSigners returns the signer address for a MsgUpdateParams message.
func (msg *MsgUpdateParams) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Authority)
	return []sdk.AccAddress{addr}
}

// ValidateBasic does a sanity check on the provided data.
func (msg *MsgUpdateParams) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Authority); err != nil {
		return errors.Wrap(err, "invalid authority address")
	}

	return msg.Params.Validate()
}

// ValidateBasic statelessly validates a root submission.
func (msg *MsgSubmitDistributionRoot) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Signer); err != nil {
		return errors.Wrap(err, "invalid signer address")
	}
	if err := validateDateString(msg.Date); err != nil {
		return err
	}
	if len(msg.MerkleRoot) == 0 {
		return fmt.Errorf("merkle root cannot be empty")
	}
	return nil
}

// ValidateBasic statelessly validates a claim: addresses parse, the total is a
// positive integer, the category breakdown is non-empty and sums to the total,
// and the proof is within the maximum depth. The handler re-checks these against
// chain state; this rejects malformed claims before they reach it.
func (msg *MsgClaim) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Claimer); err != nil {
		return errors.Wrap(err, "invalid claimer address")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Address); err != nil {
		return errors.Wrap(err, "invalid reward address")
	}
	if err := validateDateString(msg.Date); err != nil {
		return err
	}
	total, ok := math.NewIntFromString(msg.Total)
	if !ok {
		return fmt.Errorf("total is not a valid integer")
	}
	if !total.IsPositive() {
		return fmt.Errorf("total must be positive")
	}
	if len(msg.Categories) == 0 {
		return fmt.Errorf("categories must not be empty")
	}
	if len(msg.Categories) > MaxCategories {
		return fmt.Errorf("too many categories: %d exceeds max %d", len(msg.Categories), MaxCategories)
	}
	sum := math.ZeroInt()
	for category, raw := range msg.Categories {
		if category == "" {
			return fmt.Errorf("category name must not be empty")
		}
		amount, ok := math.NewIntFromString(raw)
		if !ok {
			return fmt.Errorf("category %q amount is not a valid integer", category)
		}
		if !amount.IsPositive() {
			return fmt.Errorf("category %q amount must be positive", category)
		}
		sum = sum.Add(amount)
	}
	if !sum.Equal(total) {
		return fmt.Errorf("categories sum %s does not equal total %s", sum, total)
	}
	if len(msg.Proof) > MaxProofDepth {
		return fmt.Errorf("merkle proof too long: %d exceeds max depth %d", len(msg.Proof), MaxProofDepth)
	}
	return nil
}

// ValidateBasic statelessly validates a challenge.
func (msg *MsgChallengeDistribution) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Challenger); err != nil {
		return errors.Wrap(err, "invalid challenger address")
	}
	return validateDateString(msg.Date)
}

// ValidateBasic statelessly validates a governance revival.
func (msg *MsgReviveDistribution) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Authority); err != nil {
		return errors.Wrap(err, "invalid authority address")
	}
	return validateDateString(msg.Date)
}
