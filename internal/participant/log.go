package participant

import (
	"io"
	"log/slog"
	"strings"
)

type DecisionLogger struct {
	logger  *slog.Logger
	secrets []string
}

func NewDecisionLogger(output io.Writer, secrets ...string) *DecisionLogger {
	return &DecisionLogger{logger: slog.New(slog.NewJSONHandler(output, nil)), secrets: append([]string(nil), secrets...)}
}

func (logger *DecisionLogger) Log(decision ParticipationDecision) {
	logger.logger.Info("participation decision",
		"repository", decision.Repository.Owner+"/"+decision.Repository.Name,
		"job_id", decision.JobID,
		"participant", decision.Participant,
		"decision", decision.Decision,
		"reason", Redact(decision.Reason, logger.secrets...),
		"outcome", Redact(decision.Outcome, logger.secrets...),
		"decision_time", decision.Timestamp,
	)
}

func Redact(value string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	for _, prefix := range []string{"github_pat_", "ghp_"} {
		for start := strings.Index(value, prefix); start >= 0; start = strings.Index(value, prefix) {
			end := start
			for end < len(value) && !strings.ContainsRune(" \t\r\n,;:\"'()[]{}", rune(value[end])) {
				end++
			}
			value = value[:start] + "[REDACTED]" + value[end:]
		}
	}
	return value
}
