package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const ConfigFile = "config.json"

type Config struct {
	ProjectName     string                    `json:"projectName"`
	DevRoot         string                    `json:"devRoot"`
	VaultPath       string                    `json:"vaultPath"`
	WorktreesRoot   string                    `json:"worktreesRoot"`
	DefaultProvider string                    `json:"defaultProvider"`
	Providers       map[string]ProviderConfig `json:"providers"`
}

type ProviderConfig map[string]any

func DefaultConfig(repoRoot string, devRoot string) Config {
	projectName := filepath.Base(repoRoot)
	return Config{
		ProjectName:     projectName,
		DevRoot:         devRoot,
		VaultPath:       filepath.Join(devRoot, "vaults", "AI-Vault", "10-Projects", projectName),
		WorktreesRoot:   filepath.Join(devRoot, "worktrees", "active"),
		DefaultProvider: "gemini",
		Providers:       DefaultProvidersConfig(),
	}
}

func DefaultProvidersConfig() map[string]ProviderConfig {
	return map[string]ProviderConfig{
		"codex": {
			"command":         "codex",
			"approvalMode":    "never",
			"sandboxMode":     "danger-full-access",
			"model":           "gpt-5",
			"reasoningEffort": "medium",
		},
		"gemini": {
			"command": "gemini",
			"model":   "gemini-2.5-flash",
		},
		"copilot": {
			"command":       "copilot",
			"allowAllTools": true,
			"allowAllPaths": true,
			"noAskUser":     true,
		},
	}
}

func LoadConfig(store Store) (Config, bool, error) {
	var config Config
	data, err := os.ReadFile(store.Path(ConfigFile))
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, true, nil
		}
		return Config{}, false, fmt.Errorf("read %s: %w", ConfigFile, err)
	}
	if len(data) == 0 {
		return Config{}, false, fmt.Errorf("read %s: file is empty", ConfigFile)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return Config{}, false, fmt.Errorf("parse %s: %w", ConfigFile, err)
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return Config{}, false, fmt.Errorf("parse %s: %w", ConfigFile, err)
	}
	if config.Providers == nil {
		config.Providers = map[string]ProviderConfig{}
	}
	if _, ok := raw["providers"]; !ok {
		config.Providers = map[string]ProviderConfig{}
	}
	return config, false, nil
}
