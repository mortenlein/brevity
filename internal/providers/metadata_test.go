package providers

import (
	"testing"

	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/state"
)

func TestLookupByIDAndAlias(t *testing.T) {
	if metadata, ok := Lookup("codex"); !ok || metadata.ID != "codex" {
		t.Fatalf("expected codex lookup, got %#v ok=%v", metadata, ok)
	}
	if metadata, ok := Lookup("openai"); !ok || metadata.ID != "codex" {
		t.Fatalf("expected openai alias to resolve to codex, got %#v ok=%v", metadata, ok)
	}
	if _, ok := Lookup("missing"); ok {
		t.Fatal("unknown provider unexpectedly resolved")
	}
}

func TestResolveProfileAlias(t *testing.T) {
	profile, ok := ResolveProfile("codex-default")
	if !ok {
		t.Fatal("expected profile alias to resolve")
	}
	if profile.ID != "codex-balanced" || profile.Provider != "codex" {
		t.Fatalf("unexpected profile: %#v", profile)
	}
}

func TestResolveUsesConfigDefaultProvider(t *testing.T) {
	config := RunConfig{DefaultProvider: "gemini", Providers: map[string]RuntimeConfig{}}
	resolved, warnings, blockers := Resolve(config, "", "", "", state.ProviderHealthState{"gemini": {Status: state.StatusHealthy}})
	if len(warnings) != 0 || len(blockers) != 0 {
		t.Fatalf("unexpected messages warnings=%#v blockers=%#v", warnings, blockers)
	}
	if resolved.Provider != "gemini" || resolved.Profile != "default" {
		t.Fatalf("unexpected resolution: %#v", resolved)
	}
}

func TestResolveTaskProviderOverride(t *testing.T) {
	config := RunConfig{DefaultProvider: "gemini", Providers: map[string]RuntimeConfig{}}
	resolved, _, blockers := Resolve(config, "codex", "", "", state.ProviderHealthState{"codex": {Status: state.StatusHealthy}})
	if len(blockers) != 0 {
		t.Fatalf("unexpected blockers: %#v", blockers)
	}
	if resolved.Provider != "codex" || resolved.Profile != "default" {
		t.Fatalf("unexpected resolution: %#v", resolved)
	}
}

func TestResolveExplicitProfile(t *testing.T) {
	config := RunConfig{DefaultProvider: "codex", Providers: map[string]RuntimeConfig{}}
	resolved, _, blockers := Resolve(config, "", "", "gemini-flash", state.ProviderHealthState{"gemini": {Status: state.StatusHealthy}})
	if len(blockers) != 0 {
		t.Fatalf("unexpected blockers: %#v", blockers)
	}
	if resolved.Provider != "gemini" || resolved.Profile != "gemini-flash" || resolved.Model != "gemini-3-flash-preview" {
		t.Fatalf("unexpected resolution: %#v", resolved)
	}
}

func TestResolveUnknownProviderAndProfile(t *testing.T) {
	config := RunConfig{DefaultProvider: "unknown", Providers: map[string]RuntimeConfig{}}
	_, _, blockers := Resolve(config, "", "", "", state.ProviderHealthState{})
	if !hasCode(blockers, "unsupported-provider") {
		t.Fatalf("expected unsupported-provider blocker, got %#v", blockers)
	}
	_, _, blockers = Resolve(config, "", "", "missing-profile", state.ProviderHealthState{})
	if !hasCode(blockers, "profile-not-found") {
		t.Fatalf("expected profile-not-found blocker, got %#v", blockers)
	}
}

func TestResolveProviderHealthStatuses(t *testing.T) {
	config := RunConfig{DefaultProvider: "codex", Providers: map[string]RuntimeConfig{}}
	cases := []struct {
		status state.ProviderStatus
		code   string
		block  bool
	}{
		{state.StatusUnavailable, "provider-unavailable", true},
		{state.StatusQuotaConstrained, "provider-quota-constrained", true},
		{state.StatusCapacityDegraded, "provider-capacity-degraded", false},
		{state.StatusUnknown, "provider-unknown", false},
	}
	for _, tc := range cases {
		warningsExpected := !tc.block
		_, warnings, blockers := Resolve(config, "", "", "", state.ProviderHealthState{"codex": {Status: tc.status}})
		if tc.block && !hasCode(blockers, tc.code) {
			t.Fatalf("status %s expected blocker %s, got %#v", tc.status, tc.code, blockers)
		}
		if warningsExpected && !hasCode(warnings, tc.code) {
			t.Fatalf("status %s expected warning %s, got %#v", tc.status, tc.code, warnings)
		}
	}
}

func TestDefaultConfigCompatibility(t *testing.T) {
	defaults := DefaultStateConfigProviders()
	if defaults["codex"]["approvalMode"] != "never" || defaults["gemini"]["model"] != "gemini-2.5-flash" {
		t.Fatalf("unexpected defaults: %#v", defaults)
	}
	if _, ok := defaults["copilot"]; !ok {
		t.Fatal("expected copilot compatibility default")
	}
}

func hasCode(messages []contracts.ResultMessage, code string) bool {
	for _, message := range messages {
		if message.Code == code {
			return true
		}
	}
	return false
}
