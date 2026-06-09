package types

// Event types and attribute keys emitted by the decentralized distribution.
const (
	EventTypeSubmitRoot              = "submit_distribution_root"
	EventTypeDistributionPending     = "distribution_pending"
	EventTypeDistributionUnderReview = "distribution_under_review"
	EventTypeChallengeResolved       = "distribution_challenge_resolved"
	EventTypeDistributionFinalized   = "distribution_finalized"
	EventTypeDistributionExpired     = "distribution_expired"
	EventTypeDistributionRevived     = "distribution_revived"
	EventTypeClaim                   = "claim_distribution"

	AttributeKeyDate         = "date"
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
	AttributeKeyTimedOut     = "timed_out"
)
