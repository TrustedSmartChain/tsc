package types

const (
	ModuleName = "lockup"

	StoreKey = ModuleName

	QuerierRoute = ModuleName
)

var (
	LocksByDateKey    = []byte("locks_by_date")
	LocksByAddressKey = []byte("locks_by_address")
	TotalLockedKey    = []byte("total_locked")
)
