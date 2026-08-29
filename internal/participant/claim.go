package participant

import (
	"sort"
	"strings"
	"time"
)

type claimObservation struct {
	firstSeen time.Time
	lastSeen  time.Time
}

type claimTracker struct {
	entries    map[string]claimObservation
	maxEntries int
	staleAfter time.Duration
}

func newClaimTracker(maxEntries int, staleAfter time.Duration) *claimTracker {
	return &claimTracker{entries: map[string]claimObservation{}, maxEntries: maxEntries, staleAfter: staleAfter}
}

func (tracker *claimTracker) observe(now time.Time, jobs []ObservedJob) []ObservedJob {
	for key, observation := range tracker.entries {
		if now.Sub(observation.lastSeen) > tracker.staleAfter {
			delete(tracker.entries, key)
		}
	}
	observed := make([]ObservedJob, len(jobs))
	for i, job := range jobs {
		key := jobKey(job)
		observation, exists := tracker.entries[key]
		if !exists {
			observation.firstSeen = now
		}
		observation.lastSeen = now
		tracker.entries[key] = observation
		job.FirstSeenAt = observation.firstSeen
		observed[i] = job
	}
	for len(tracker.entries) > tracker.maxEntries {
		oldestKey := ""
		var oldest time.Time
		for key, observation := range tracker.entries {
			if oldestKey == "" || observation.lastSeen.Before(oldest) {
				oldestKey, oldest = key, observation.lastSeen
			}
		}
		delete(tracker.entries, oldestKey)
	}
	return observed
}

func eligibleClaims(now time.Time, delay time.Duration, jobs []ObservedJob) []ObservedJob {
	eligible := make([]ObservedJob, 0, len(jobs))
	for _, job := range jobs {
		if !now.Before(job.FirstSeenAt.Add(delay)) {
			eligible = append(eligible, job)
		}
	}
	sort.Slice(eligible, func(i, j int) bool {
		left, right := eligible[i], eligible[j]
		leftEligible, rightEligible := left.FirstSeenAt.Add(delay), right.FirstSeenAt.Add(delay)
		if !leftEligible.Equal(rightEligible) {
			return leftEligible.Before(rightEligible)
		}
		if repository := strings.Compare(strings.ToLower(left.Repository.Owner+"/"+left.Repository.Name), strings.ToLower(right.Repository.Owner+"/"+right.Repository.Name)); repository != 0 {
			return repository < 0
		}
		if left.RunID != right.RunID {
			return left.RunID < right.RunID
		}
		return left.JobID < right.JobID
	})
	return eligible
}
