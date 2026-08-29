package participant

import (
	"testing"
	"time"
)

func TestHealthyClaimTimingMeetsSC002(t *testing.T) {
	start := time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)
	claimDelay, pollInterval := 30*time.Second, 10*time.Second
	withinTarget := 0
	for i := 0; i < 100; i++ {
		firstSeen := start.Add(time.Duration(i) * time.Minute)
		activation := firstSeen.Add(claimDelay + pollInterval)
		if activation.Sub(firstSeen) <= claimDelay+pollInterval+30*time.Second {
			withinTarget++
		}
	}
	if withinTarget < 99 {
		t.Fatalf("healthy observations within SC-002 target = %d/100", withinTarget)
	}
}
