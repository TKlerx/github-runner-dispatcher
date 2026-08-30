package participant

import "time"

type Repository struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
}

type ObservedJob struct {
	Repository        Repository
	ServerRepository  string
	RepositoryPrivate bool
	RunID             int64
	JobID             int64
	Name              string
	Status            string
	Labels            []string
	RunnerName        string
	WorkflowID        int64
	WorkflowPath      string
	Event             string
	Actor             string
	TriggeringActor   string
	FirstSeenAt       time.Time
}

type Decision string

const (
	DecisionIgnore    Decision = "ignore"
	DecisionWait      Decision = "wait"
	DecisionOffer     Decision = "offer"
	DecisionAdopt     Decision = "adopt"
	DecisionTerminate Decision = "terminate"
	DecisionCleanup   Decision = "cleanup"
	DecisionError     Decision = "error"
)

type ParticipationDecision struct {
	Repository  Repository
	JobID       int64
	Participant string
	Decision    Decision
	Reason      string
	Timestamp   time.Time
	Outcome     string
}
