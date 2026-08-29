package participant

import (
	"testing"
	"time"
)

func TestClaimTrackerPreservesFirstSeenAndAppliesDelay(t *testing.T) {
	tracker := newClaimTracker(10, time.Minute)
	start := time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)
	job := ObservedJob{Repository: Repository{Owner: "TKlerx", Name: "repo"}, RunID: 1, JobID: 2, Status: "queued"}

	observed := tracker.observe(start, []ObservedJob{job})
	observed = tracker.observe(start.Add(5*time.Second), observed)
	if !observed[0].FirstSeenAt.Equal(start) {
		t.Fatalf("first seen = %s", observed[0].FirstSeenAt)
	}
	if eligible := eligibleClaims(start.Add(9*time.Second), 10*time.Second, observed); len(eligible) != 0 {
		t.Fatalf("job became eligible early: %#v", eligible)
	}
	if eligible := eligibleClaims(start.Add(10*time.Second), 10*time.Second, observed); len(eligible) != 1 {
		t.Fatalf("eligible jobs = %#v", eligible)
	}
}

func TestClaimTrackerEvictsStaleObservations(t *testing.T) {
	tracker := newClaimTracker(10, 10*time.Second)
	start := time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)
	old := ObservedJob{Repository: Repository{Owner: "TKlerx", Name: "old"}, JobID: 1, Status: "queued"}
	tracker.observe(start, []ObservedJob{old})
	tracker.observe(start.Add(11*time.Second), nil)
	observed := tracker.observe(start.Add(12*time.Second), []ObservedJob{old})
	if !observed[0].FirstSeenAt.Equal(start.Add(12 * time.Second)) {
		t.Fatalf("stale first-seen timestamp survived: %s", observed[0].FirstSeenAt)
	}
}

func TestEligibleClaimsOrdersByEligibilityThenRepositoryAndIDs(t *testing.T) {
	start := time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)
	jobs := []ObservedJob{
		{Repository: Repository{Owner: "TKlerx", Name: "zeta"}, RunID: 1, JobID: 1, FirstSeenAt: start},
		{Repository: Repository{Owner: "TKlerx", Name: "alpha"}, RunID: 2, JobID: 3, FirstSeenAt: start},
		{Repository: Repository{Owner: "TKlerx", Name: "alpha"}, RunID: 1, JobID: 2, FirstSeenAt: start},
		{Repository: Repository{Owner: "TKlerx", Name: "earlier"}, RunID: 9, JobID: 9, FirstSeenAt: start.Add(-time.Second)},
	}
	eligible := eligibleClaims(start.Add(time.Minute), 10*time.Second, jobs)
	want := []int64{9, 2, 3, 1}
	for i := range want {
		if eligible[i].JobID != want[i] {
			t.Fatalf("eligible order = %#v", eligible)
		}
	}
}
