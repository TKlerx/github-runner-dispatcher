package participant

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestDecisionLoggerProducesStructuredSecretFreeOutput(t *testing.T) {
	const token = "github_pat_secret-token"
	const jit = "encoded-jit-secret"
	var output bytes.Buffer
	logger := NewDecisionLogger(&output, token, jit)
	logger.Log(ParticipationDecision{
		Repository: Repository{Owner: "TKlerx", Name: "repo"}, JobID: 42, Participant: "jan-cachy",
		Decision: DecisionError, Reason: "PAT " + token + " failed", Outcome: "JIT " + jit + " rejected", Timestamp: time.Unix(1, 0),
	})

	text := output.String()
	if strings.Contains(text, token) || strings.Contains(text, jit) {
		t.Fatalf("secret leaked: %s", text)
	}
	for _, field := range []string{`"repository":"TKlerx/repo"`, `"job_id":42`, `"participant":"jan-cachy"`, `"decision":"error"`, `"reason":"PAT [REDACTED] failed"`} {
		if !strings.Contains(text, field) {
			t.Fatalf("missing %s in %s", field, text)
		}
	}
}

func TestRedactMasksKnownGitHubTokenPrefixes(t *testing.T) {
	for _, secret := range []string{"ghp_abcdefghijklmnopqrstuvwxyz", "github_pat_abcdefghijklmnopqrstuvwxyz"} {
		if redacted := Redact("failed " + secret); strings.Contains(redacted, secret) {
			t.Fatalf("Redact() leaked %s", secret)
		}
	}
}
