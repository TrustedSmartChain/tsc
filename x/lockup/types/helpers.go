package types

import "time"

func IsLocked(currentDate time.Time, unlockDate string) bool {
	unlockTime, err := time.Parse(time.DateOnly, unlockDate)
	if err != nil {
		return false
	}

	return currentDate.Before(unlockTime)
}
