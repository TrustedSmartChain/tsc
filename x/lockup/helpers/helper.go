package helpers

import "time"

func IsLocked(currentTime time.Time, unlockDate string) bool {
	unlockTime, err := time.Parse(time.DateOnly, unlockDate)
	if err != nil {
		return false
	}
	return currentTime.Truncate(24 * time.Hour).Before(unlockTime)
}
