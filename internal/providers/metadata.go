package providers

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/state"
)

type Kind string

const (
	KindNativeCLI Kind = "native-cli"
	KindLegacyCLI Kind = "legacy-cli"
)

type RuntimeConfig struct {
	Command         string            `json:"command"`
	Mode            string            `json:"mode"`
	Sandbox         string            `json:"sandbox"`
	Model           string            `json:"model"`
	Profile         string            `json:"profile"`
	ExecutionPolicy string            `json:"executionPolicy"`
	ApprovalMode    string            `json:"approvalMode"`
	SkipTrust       bool              `json:"skipTrust"`
	Env             map[string]string `json:"env"`
}

type Profile struct {
	ID       string
	Aliases  []string
	Provider string
	Model    string
}

type Metadata struct {
	ID                              string
	DisplayName                     string
	Aliases                         []string
	DefaultProfile                  string
	SupportedProfiles               []string
	CommandExecutable               string
	DefaultArgs                     []string
	EnvVarRequirements              []string
	Kind                            Kind
	SupportsStreaming               bool
	SupportsNonInteractiveExecution bool
	SupportsApprovalMode            bool
	SupportsSkipTrust               bool
	DefaultModel                    string
	SafetyNotes                     []string
	DefaultConfig                   RuntimeConfig
	Supported                       bool
}

type RunConfig struct {
	DefaultProvider string                   `json:"defaultProvider"`
	Providers       map[string]RuntimeConfig `json:"providers"`
	Codex           *RuntimeConfig           `json:"codex"`
}

type Resolved struct {
	Provider string
	Profile  string
	Model    string
	Config   RuntimeConfig
	Metadata Metadata
	Health   state.ProviderHealth
}

var registry = []Metadata{
	{
		ID:                              "codex",
		DisplayName:                     "Codex",
		Aliases:                         []string{"openai"},
		DefaultProfile:                  "codex-balanced",
		SupportedProfiles:               []string{"codex-fast", "codex-balanced", "codex-deep"},
		CommandExecutable:               "codex",
		Kind:                            KindNativeCLI,
		SupportsNonInteractiveExecution: true,
		DefaultModel:                    "gpt-5",
		SafetyNotes:                     []string{"Codex execution is provider-running and must remain plan-gated."},
		DefaultConfig:                   RuntimeConfig{Command: "codex", Mode: "exec", Sandbox: "workspace-write", ExecutionPolicy: "Bypass", Env: map[string]string{}},
		Supported:                       true,
	},
	{
		ID:                              "gemini",
		DisplayName:                     "Gemini",
		Aliases:                         []string{"google-gemini"},
		DefaultProfile:                  "gemini-flash",
		SupportedProfiles:               []string{"gemini-lite", "gemini-flash", "gemini-pro"},
		CommandExecutable:               "gemini",
		Kind:                            KindNativeCLI,
		SupportsNonInteractiveExecution: true,
		SupportsApprovalMode:            true,
		SupportsSkipTrust:               true,
		DefaultModel:                    "gemini-3-flash-preview",
		SafetyNotes:                     []string{"Gemini yolo/skip-trust options are compatibility defaults, not automatic approval to execute."},
		DefaultConfig:                   RuntimeConfig{Command: "gemini", Sandbox: "workspace-write", ApprovalMode: "yolo", SkipTrust: true, Env: map[string]string{}},
		Supported:                       true,
	},
	{
		ID:                              "antigravity",
		DisplayName:                     "Antigravity",
		DefaultProfile:                  "antigravity",
		SupportedProfiles:               []string{"antigravity"},
		CommandExecutable:               "antigravity",
		Kind:                            KindNativeCLI,
		SupportsNonInteractiveExecution: true,
		SafetyNotes:                     []string{"Antigravity planning is supported, execution semantics must stay unchanged."},
		DefaultConfig:                   RuntimeConfig{Command: "antigravity", Env: map[string]string{}},
		Supported:                       true,
	},
	{
		ID:                              "copilot",
		DisplayName:                     "Copilot",
		DefaultProfile:                  "copilot",
		SupportedProfiles:               []string{"copilot"},
		CommandExecutable:               "copilot",
		Kind:                            KindLegacyCLI,
		SupportsNonInteractiveExecution: true,
		SafetyNotes:                     []string{"Copilot remains a compatibility/default-config provider and is not supported by native task-run planning."},
		DefaultConfig:                   RuntimeConfig{Command: "copilot", Env: map[string]string{}},
		Supported:                       false,
	},
}

var profiles = map[string]Profile{
	"gemini-lite":    {ID: "gemini-lite", Provider: "gemini"},
	"gemini-flash":   {ID: "gemini-flash", Provider: "gemini", Model: "gemini-3-flash-preview"},
	"gemini-pro":     {ID: "gemini-pro", Provider: "gemini"},
	"codex-fast":     {ID: "codex-fast", Provider: "codex"},
	"codex-balanced": {ID: "codex-balanced", Provider: "codex"},
	"codex-deep":     {ID: "codex-deep", Provider: "codex"},
	"antigravity":    {ID: "antigravity", Provider: "antigravity"},
	"copilot":        {ID: "copilot", Provider: "copilot"},
}

var profileAliases = map[string]string{
	"gemini-fast":     "gemini-flash",
	"gemini-balanced": "gemini-pro",
	"gemini-default":  "gemini-flash",
	"codex-default":   "codex-balanced",
	"codex-standard":  "codex-balanced",
	"codex-pro":       "codex-deep",
}

func All() []Metadata {
	result := make([]Metadata, len(registry))
	copy(result, registry)
	return result
}

func Lookup(provider string) (Metadata, bool) {
	normalized := normalize(provider)
	for _, metadata := range registry {
		if metadata.ID == normalized {
			return metadata, true
		}
		for _, alias := range metadata.Aliases {
			if normalize(alias) == normalized {
				return metadata, true
			}
		}
	}
	return Metadata{}, false
}

func ResolveProfile(name string) (Profile, bool) {
	normalized := normalize(name)
	if alias := profileAliases[normalized]; alias != "" {
		normalized = alias
	}
	profile, ok := profiles[normalized]
	return profile, ok
}

func ReadRunConfig(store state.Store) (RunConfig, []contracts.ResultMessage) {
	config := RunConfig{DefaultProvider: "codex", Providers: map[string]RuntimeConfig{}}
	data, err := os.ReadFile(store.Path(state.ConfigFile))
	if err != nil {
		if os.IsNotExist(err) {
			return config, []contracts.ResultMessage{{Code: "config-missing", Message: ".brevity/config.json is missing; native defaults were used."}}
		}
		return config, []contracts.ResultMessage{{Code: "config-read-error", Message: err.Error()}}
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return RunConfig{DefaultProvider: "codex", Providers: map[string]RuntimeConfig{}}, []contracts.ResultMessage{{Code: "config-parse-error", Message: err.Error()}}
	}
	if config.Providers == nil {
		config.Providers = map[string]RuntimeConfig{}
	}
	return config, nil
}

func Resolve(config RunConfig, taskProvider string, taskProfile string, requestedProfile string, health state.ProviderHealthState) (Resolved, []contracts.ResultMessage, []contracts.ResultMessage) {
	warnings := []contracts.ResultMessage{}
	blockers := []contracts.ResultMessage{}
	provider := normalize(firstNonEmpty(taskProvider, config.DefaultProvider, "codex"))
	profileID := firstNonEmpty(taskProfile, "default")
	model := ""
	if requestedProfile != "" {
		profile, ok := ResolveProfile(requestedProfile)
		if !ok {
			return Resolved{Provider: provider, Profile: requestedProfile}, nil, []contracts.ResultMessage{{Code: "profile-not-found", Message: "Unknown worker profile: " + requestedProfile}}
		}
		provider = profile.Provider
		profileID = profile.ID
		model = profile.Model
	}
	metadata, ok := Lookup(provider)
	if !ok {
		metadata = Metadata{ID: provider, DefaultConfig: RuntimeConfig{Env: map[string]string{}}}
		blockers = append(blockers, contracts.ResultMessage{Code: "unsupported-provider", Message: "Unsupported worker provider: " + provider})
	} else if !metadata.Supported {
		blockers = append(blockers, contracts.ResultMessage{Code: "unsupported-provider", Message: "Unsupported worker provider: " + provider})
	}
	if requestedProfile == "" && profileID != "" && profileID != "default" {
		if profile, ok := ResolveProfile(profileID); ok {
			model = profile.Model
			if profile.Provider != "" {
				provider = profile.Provider
			}
		}
	}
	cfg := metadata.DefaultConfig
	if cfg.Env == nil {
		cfg.Env = map[string]string{}
	}
	if provider == "codex" && config.Codex != nil {
		cfg = MergeConfig(cfg, *config.Codex)
	}
	if providerConfig, ok := config.Providers[provider]; ok {
		cfg = MergeConfig(cfg, providerConfig)
	}
	if model == "" {
		model = cfg.Model
	}
	if cfg.Command == "" {
		cfg.Command = metadata.CommandExecutable
	}
	if cfg.Command == "" {
		blockers = append(blockers, contracts.ResultMessage{Code: "missing-provider-command", Message: "Provider command is not configured."})
	}
	if len(cfg.Env) > 0 {
		warnings = append(warnings, contracts.ResultMessage{Code: "provider-env-redacted", Message: "Provider environment values are redacted in the plan.", Count: len(cfg.Env)})
	}
	resolved := Resolved{Provider: provider, Profile: profileID, Model: model, Config: cfg, Metadata: metadata, Health: health[provider]}
	warnings = append(warnings, HealthMessages(provider, resolved.Health, false)...)
	blockers = append(blockers, HealthMessages(provider, resolved.Health, true)...)
	return resolved, warnings, blockers
}

func HealthMessages(provider string, record state.ProviderHealth, blockers bool) []contracts.ResultMessage {
	switch record.Status {
	case state.StatusUnavailable:
		if blockers {
			return []contracts.ResultMessage{{Code: "provider-unavailable", Message: "Provider '" + provider + "' is currently unavailable.", Details: map[string]any{"provider": provider, "status": string(record.Status), "note": record.Note}}}
		}
	case state.StatusQuotaConstrained:
		if blockers {
			return []contracts.ResultMessage{{Code: "provider-quota-constrained", Message: "Provider '" + provider + "' is quota-constrained.", Details: map[string]any{"provider": provider, "status": string(record.Status), "note": record.Note}}}
		}
	case state.StatusCapacityDegraded, state.StatusUnknown, "":
		if !blockers {
			return []contracts.ResultMessage{{Code: "provider-" + firstNonEmpty(string(record.Status), "unknown"), Message: "Provider '" + provider + "' readiness is degraded or unknown.", Details: map[string]any{"provider": provider, "status": string(record.Status), "note": record.Note}}}
		}
	}
	return nil
}

func DefaultStateConfigProviders() map[string]state.ProviderConfig {
	result := map[string]state.ProviderConfig{}
	for _, metadata := range registry {
		cfg := state.ProviderConfig{"command": metadata.CommandExecutable}
		switch metadata.ID {
		case "codex":
			cfg["approvalMode"] = "never"
			cfg["sandboxMode"] = "danger-full-access"
			cfg["model"] = metadata.DefaultModel
			cfg["reasoningEffort"] = "medium"
		case "gemini":
			cfg["model"] = "gemini-2.5-flash"
		case "copilot":
			cfg["allowAllTools"] = true
			cfg["allowAllPaths"] = true
			cfg["noAskUser"] = true
		default:
			continue
		}
		result[metadata.ID] = cfg
	}
	return result
}

func DefaultHealthState() state.ProviderHealthState {
	result := state.ProviderHealthState{}
	for _, metadata := range registry {
		if metadata.ID == "antigravity" {
			continue
		}
		result[metadata.ID] = state.ProviderHealth{Status: state.StatusUnknown, Note: "", UpdatedAt: ""}
	}
	return result
}

func MergeConfig(base RuntimeConfig, override RuntimeConfig) RuntimeConfig {
	if override.Command != "" {
		base.Command = override.Command
	}
	if override.Mode != "" {
		base.Mode = override.Mode
	}
	if override.Sandbox != "" {
		base.Sandbox = override.Sandbox
	}
	if override.Model != "" {
		base.Model = override.Model
	}
	if override.Profile != "" {
		base.Profile = override.Profile
	}
	if override.ExecutionPolicy != "" {
		base.ExecutionPolicy = override.ExecutionPolicy
	}
	if override.ApprovalMode != "" {
		base.ApprovalMode = override.ApprovalMode
	}
	if override.SkipTrust {
		base.SkipTrust = true
	}
	if override.Env != nil {
		base.Env = override.Env
	}
	return base
}

func SupportedProvider(provider string) bool {
	metadata, ok := Lookup(provider)
	return ok && metadata.Supported
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func UnknownProfileError(profile string) error {
	return fmt.Errorf("unknown worker profile: %s", profile)
}
