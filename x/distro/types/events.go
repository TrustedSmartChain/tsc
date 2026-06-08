package types

// Event types and attribute keys emitted by the decentralized distribution.
const (
	EventTypeSubmitRoot        = "submit_distribution_root"
	EventTypeEpochPending      = "epoch_distribution_pending"
	EventTypeEpochUnderReview  = "epoch_distribution_under_review"
	EventTypeChallengeResolved = "epoch_distribution_challenge_resolved"
	EventTypeEpochFinalized    = "epoch_distribution_finalized"
	EventTypeEpochExpired      = "epoch_distribution_expired"
	EventTypeClaim             = "claim_distribution"

	AttributeKeyEpoch        = "epoch"
	AttributeKeySigner       = "signer"
	AttributeKeyMerkleRoot   = "merkle_root"
	AttributeKeyLicenseTally = "license_tally"
	AttributeKeyStakeTally   = "stake_tally"
	AttributeKeyNonce        = "nonce"
	AttributeKeyClaimer      = "claimer"
	AttributeKeyAddress      = "address"
	AttributeKeyAmount       = "amount"
	AttributeKeyChallenger   = "challenger"
	AttributeKeyBond         = "bond"
	AttributeKeyFrivolous    = "frivolous"
)
