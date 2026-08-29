package participant

import (
	"reflect"
	"testing"
)

func TestMatchingQueuedJobsUsesCaseInsensitiveLabelSubset(t *testing.T) {
	jobs := []ObservedJob{
		{Repository: Repository{Owner: "TKlerx", Name: "repo"}, RunID: 2, JobID: 3, Status: "queued", Labels: []string{"SELF-HOSTED", "windows", "x64"}},
		{Repository: Repository{Owner: "TKlerx", Name: "repo"}, RunID: 2, JobID: 4, Status: "queued", Labels: []string{"self-hosted", "Linux", "X64"}},
		{Repository: Repository{Owner: "TKlerx", Name: "repo"}, RunID: 2, JobID: 5, Status: "in_progress", Labels: []string{"self-hosted", "Windows", "X64"}},
	}

	matched := matchingQueuedJobs(jobs, []string{"self-hosted", "Windows", "X64", "strong"})
	if len(matched) != 1 || matched[0].JobID != 3 {
		t.Fatalf("matched = %#v", matched)
	}
}

func TestMatchingQueuedJobsOrdersDeterministically(t *testing.T) {
	jobs := []ObservedJob{
		{Repository: Repository{Owner: "TKlerx", Name: "zeta"}, RunID: 1, JobID: 1, Status: "queued", Labels: []string{"self-hosted"}},
		{Repository: Repository{Owner: "TKlerx", Name: "alpha"}, RunID: 2, JobID: 3, Status: "queued", Labels: []string{"self-hosted"}},
		{Repository: Repository{Owner: "TKlerx", Name: "alpha"}, RunID: 1, JobID: 2, Status: "queued", Labels: []string{"self-hosted"}},
	}

	matched := matchingQueuedJobs(jobs, []string{"self-hosted"})
	ids := []int64{matched[0].JobID, matched[1].JobID, matched[2].JobID}
	if !reflect.DeepEqual(ids, []int64{2, 3, 1}) {
		t.Fatalf("job order = %v", ids)
	}
}

func TestLabelsMatchRejectsUnmatchedOperatingSystem(t *testing.T) {
	if labelsMatch([]string{"self-hosted", "Linux", "X64"}, []string{"self-hosted", "Windows", "X64"}) {
		t.Fatal("Windows participant matched a Linux job")
	}
}
