package types

import (
	"time"

	networktypes "github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// SameDay reports whether a and b fall in the same UTC day bucket. It
// delegates to the network module's day-bucket helpers so both modules roll
// their daily counters on identical boundaries.
func SameDay(a, b time.Time) bool {
	return networktypes.SameDay(a, b)
}
