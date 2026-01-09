package types

const (
	EventTypeLock         = "lock"
	EventTypeLockExtended = "lock_extended"
	EventTypeLockExpired  = "lock_expired"

	AttributeKeyLockAddress   = "address"
	AttributeKeyAmount        = "amount"
	AttributeKeyUnlockDate    = "unlock_date"
	AttributeKeyOldUnlockDate = "old_unlock_date"
)
