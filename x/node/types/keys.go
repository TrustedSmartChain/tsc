package types

const (
	ModuleName   = "node"
	StoreKey     = ModuleName
	QuerierRoute = ModuleName

	// CheckInKeyPrefix is the KV prefix for check-in records: checkin/{address}/{date}
	CheckInKeyPrefix = "checkin/"
)

// CheckInKey returns the store key for a given address and UTC date (YYYY-MM-DD).
func CheckInKey(address, date string) []byte {
	return []byte(CheckInKeyPrefix + address + "/" + date)
}

// CheckInAddressPrefix returns the prefix for all check-ins by a given address.
func CheckInAddressPrefix(address string) []byte {
	return []byte(CheckInKeyPrefix + address + "/")
}
