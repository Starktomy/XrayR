package rule

import (
	"regexp"
	"testing"

	"github.com/Starktomy/XrayR/api"
)

func TestNew(t *testing.T) {
	m := New()
	if m == nil {
		t.Fatal("New returned nil")
	}
	if m.InboundRule == nil {
		t.Error("InboundRule should be initialized")
	}
	if m.InboundDetectResult == nil {
		t.Error("InboundDetectResult should be initialized")
	}
}

func TestUpdateRuleIdempotent(t *testing.T) {
	m := New()
	rules := []api.DetectRule{
		{ID: 1, Pattern: regexp.MustCompile("^bad-pattern-1$")},
		{ID: 2, Pattern: regexp.MustCompile("^bad-pattern-2$")},
	}
	if err := m.UpdateRule("tag-a", rules); err != nil {
		t.Fatalf("UpdateRule: %s", err)
	}
	// Calling UpdateRule again with the same rules should not
	// panic or change behavior.
	if err := m.UpdateRule("tag-a", rules); err != nil {
		t.Fatalf("UpdateRule (idempotent): %s", err)
	}
	v, ok := m.InboundRule.Load("tag-a")
	if !ok {
		t.Fatal("expected rule list to be stored")
	}
	stored := v.([]api.DetectRule)
	if len(stored) != 2 {
		t.Errorf("expected 2 rules, got %d", len(stored))
	}
}

func TestDetect(t *testing.T) {
	m := New()
	rules := []api.DetectRule{
		{ID: 7, Pattern: regexp.MustCompile("^blocked\\.example\\.com$")},
	}
	if err := m.UpdateRule("tag-a", rules); err != nil {
		t.Fatalf("UpdateRule: %s", err)
	}
	// A non-matching destination should be accepted.
	if m.Detect("tag-a", "allowed.example.com", "alice|1") {
		t.Errorf("Detect should not match an unrelated domain")
	}
	// A matching destination should be rejected.
	if !m.Detect("tag-a", "blocked.example.com", "alice|1") {
		t.Errorf("Detect should match the blocked domain")
	}
}

func TestDetectNoRules(t *testing.T) {
	m := New()
	// No rule stored for this tag -> never reject.
	if m.Detect("missing-tag", "anything", "alice|1") {
		t.Error("Detect should return false when no rule is registered")
	}
}

func TestGetDetectResultEmpty(t *testing.T) {
	m := New()
	out, err := m.GetDetectResult("never-stored")
	if err != nil {
		t.Fatalf("GetDetectResult: %s", err)
	}
	if out == nil {
		t.Fatal("expected non-nil empty slice")
	}
	if len(*out) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(*out))
	}
}
