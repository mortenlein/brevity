package actions

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mortenlein/brevity/internal/contracts"
	"github.com/mortenlein/brevity/internal/providers"
	"github.com/mortenlein/brevity/internal/state"
	"github.com/mortenlein/brevity/internal/state/locking"
)

const initCommand = "init"

type InitService struct {
	Store       state.Store
	Repair      bool
	LockOptions locking.Options
}

type initItem struct {
	Status string `json:"status"`
	Path   string `json:"path"`
}

type repairField struct {
	Status   string `json:"status"`
	Name     string `json:"name"`
	OldValue any    `json:"oldValue,omitempty"`
	NewValue any    `json:"newValue,omitempty"`
}

func (service InitService) Run() (contracts.CommandResult, error) {
	repoRoot, err := gitRepoRoot(service.Store.RepoRoot)
	if err != nil {
		return initError("not-git-repository", "Brevity must be run inside a Git repository.", nil)
	}
	store := service.Store
	store.RepoRoot = repoRoot
	devRoot := filepath.Dir(repoRoot)
	defaultConfig := state.DefaultConfig(repoRoot, devRoot)
	defaultConfig.Providers = providers.DefaultStateConfigProviders()
	effectiveConfig := defaultConfig
	items := []initItem{}
	fields := []repairField{}

	brevityRootExisted := localPathExists(store.BrevityRoot())
	if err := os.MkdirAll(store.BrevityRoot(), 0o755); err != nil {
		return initError("create-brevity-root-failed", err.Error(), map[string]any{"path": store.BrevityRoot()})
	}
	items = append(items, initItem{Status: createdStatus(brevityRootExisted), Path: store.BrevityRoot()})
	lockOptions := service.LockOptions
	if lockOptions.Timeout == 0 {
		lockOptions.Timeout = 5 * time.Second
	}
	lock, err := locking.Acquire(store.LockPath(), lockOptions)
	if err != nil {
		return initError("state-lock-failed", err.Error(), map[string]any{"lockPath": store.LockPath()})
	}
	defer lock.Release()

	tasksExisted := localPathExists(store.Path(state.TasksFile))
	if !tasksExisted {
		if err := store.WriteJSON(state.TasksFile, []state.Task{}); err != nil {
			return initError("tasks-create-failed", err.Error(), map[string]any{"path": store.Path(state.TasksFile)})
		}
	}
	items = append(items, initItem{Status: createdStatus(tasksExisted), Path: store.Path(state.TasksFile)})
	if existed, err := ensureProviderHealth(store); err != nil {
		return initError("provider-health-create-failed", err.Error(), map[string]any{"path": store.Path(state.ProviderHealthFile)})
	} else {
		items = append(items, initItem{Status: createdStatus(existed), Path: store.Path(state.ProviderHealthFile)})
	}

	if service.Repair {
		config, missing, err := state.LoadConfig(store)
		if err != nil {
			config = state.Config{}
		}
		config, fields = repairConfig(config, defaultConfig)
		effectiveConfig = config
		if err := store.WriteJSON(state.ConfigFile, config); err != nil {
			return initError("config-repair-failed", err.Error(), map[string]any{"path": store.Path(state.ConfigFile)})
		}
		status := "existing"
		if missing {
			status = "created"
		}
		items = append(items, initItem{Status: status, Path: store.Path(state.ConfigFile)})
	} else {
		configExisted := localPathExists(store.Path(state.ConfigFile))
		if !configExisted {
			if err := store.WriteJSON(state.ConfigFile, defaultConfig); err != nil {
				return initError("config-create-failed", err.Error(), map[string]any{"path": store.Path(state.ConfigFile)})
			}
		}
		items = append(items, initItem{Status: createdStatus(configExisted), Path: store.Path(state.ConfigFile)})
	}

	for _, dir := range []string{effectiveConfig.VaultPath, filepath.Join(effectiveConfig.VaultPath, "session-notes"), filepath.Join(effectiveConfig.VaultPath, "tasks")} {
		existed := localPathExists(dir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return initError("vault-create-failed", err.Error(), map[string]any{"path": dir})
		}
		items = append(items, initItem{Status: createdStatus(existed), Path: dir})
	}
	files := map[string]string{
		filepath.Join(effectiveConfig.VaultPath, "project.md"):      "# " + effectiveConfig.ProjectName + "\n\nProject memory for Brevity-assisted work.\n",
		filepath.Join(effectiveConfig.VaultPath, "architecture.md"): "# Architecture\n\nRecord durable architecture context here.\n",
		filepath.Join(effectiveConfig.VaultPath, "decisions.md"):    "# Decisions\n\nRecord durable project decisions here.\n",
		filepath.Join(repoRoot, "AGENTS.md"):                        "# Agent Instructions\n\nBefore doing work in this repository, read the project memory in:\n\n```text\n" + effectiveConfig.VaultPath + "\n```\n\nUse the vault memory for durable project context. Do not overwrite existing repository files unless the task explicitly requires it.\n",
	}
	paths := sortedKeys(files)
	for _, path := range paths {
		if existed, err := ensureTextFile(path, files[path]); err != nil {
			return initError("file-create-failed", err.Error(), map[string]any{"path": path})
		} else {
			items = append(items, initItem{Status: createdStatus(existed), Path: path})
		}
	}

	payload := contracts.InitPayload{
		ProjectName:         effectiveConfig.ProjectName,
		RepoRoot:            repoRoot,
		DevRoot:             effectiveConfig.DevRoot,
		VaultPath:           effectiveConfig.VaultPath,
		WorktreesRoot:       effectiveConfig.WorktreesRoot,
		Repair:              service.Repair,
		Items:               toInitPayloadItems(items),
		RepairFields:        toRepairPayloadFields(fields),
		NoProviderExecution: true,
		NoWorkerExecution:   true,
	}
	raw, _ := json.Marshal(payload)
	return contracts.CommandResult{Schema: contracts.CommandResultSchema, Command: initCommand, Success: true, Severity: "info", Warnings: []contracts.ResultMessage{}, Errors: []contracts.ResultMessage{}, SuggestedNextActions: []string{"review-config", "run-doctor"}, Payload: raw}, nil
}

func repairConfig(config state.Config, defaults state.Config) (state.Config, []repairField) {
	fields := []repairField{}
	setString := func(name string, current *string, expected string) {
		old := *current
		status := "unchanged"
		if strings.TrimSpace(old) == "" {
			*current = expected
			status = "repaired"
		}
		fields = append(fields, repairField{Status: status, Name: name, OldValue: old, NewValue: expected})
	}
	setString("projectName", &config.ProjectName, defaults.ProjectName)
	setString("devRoot", &config.DevRoot, defaults.DevRoot)
	setString("vaultPath", &config.VaultPath, defaults.VaultPath)
	setString("worktreesRoot", &config.WorktreesRoot, defaults.WorktreesRoot)
	if strings.TrimSpace(config.DefaultProvider) == "" {
		config.DefaultProvider = defaults.DefaultProvider
		fields = append(fields, repairField{Status: "repaired", Name: "defaultProvider", NewValue: defaults.DefaultProvider})
	} else {
		fields = append(fields, repairField{Status: "unchanged", Name: "defaultProvider", OldValue: config.DefaultProvider, NewValue: config.DefaultProvider})
	}
	if config.Providers == nil {
		config.Providers = map[string]state.ProviderConfig{}
		fields = append(fields, repairField{Status: "repaired", Name: "providers", NewValue: "[object]"})
	}
	for name, provider := range defaults.Providers {
		if _, ok := config.Providers[name]; !ok {
			config.Providers[name] = provider
			fields = append(fields, repairField{Status: "repaired", Name: "providers." + name, NewValue: "[object]"})
		} else {
			fields = append(fields, repairField{Status: "unchanged", Name: "providers." + name, NewValue: "[object]"})
		}
	}
	return config, fields
}

func ensureProviderHealth(store state.Store) (bool, error) {
	path := store.Path(state.ProviderHealthFile)
	existed := localPathExists(path)
	if existed {
		health, missing, err := state.LoadProviderHealth(store)
		if err != nil || missing {
			return existed, err
		}
		changed := false
		for provider, value := range providers.DefaultHealthState() {
			if _, ok := health[provider]; !ok {
				health[provider] = value
				changed = true
			}
		}
		if changed {
			return existed, store.WriteJSON(state.ProviderHealthFile, health)
		}
		return existed, nil
	}
	return existed, store.WriteJSON(state.ProviderHealthFile, providers.DefaultHealthState())
}

func gitRepoRoot(start string) (string, error) {
	if strings.TrimSpace(start) == "" {
		start = "."
	}
	command := exec.Command("git", "-C", start, "rev-parse", "--show-toplevel")
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	root := strings.TrimSpace(string(output))
	if root == "" || strings.HasPrefix(root, "-") {
		return "", errors.New("git returned no repository root")
	}
	return filepath.Clean(root), nil
}

func ensureJSONFile(path string, contents []byte) (bool, error) {
	existed := localPathExists(path)
	if existed {
		return true, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return false, os.WriteFile(path, contents, 0o644)
}

func ensureTextFile(path string, contents string) (bool, error) {
	return ensureJSONFile(path, []byte(contents))
}

func localPathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func createdStatus(existed bool) string {
	if existed {
		return "existing"
	}
	return "created"
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func toInitPayloadItems(items []initItem) []contracts.InitItem {
	result := make([]contracts.InitItem, 0, len(items))
	for _, item := range items {
		result = append(result, contracts.InitItem{Status: item.Status, Path: item.Path})
	}
	return result
}

func toRepairPayloadFields(fields []repairField) []contracts.RepairField {
	result := make([]contracts.RepairField, 0, len(fields))
	for _, field := range fields {
		result = append(result, contracts.RepairField{Status: field.Status, Name: field.Name, OldValue: field.OldValue, NewValue: field.NewValue})
	}
	return result
}

func RenderInitResult(stdout io.Writer, result contracts.CommandResult) error {
	payload, err := contracts.ParseInitPayload(result)
	if err != nil {
		return err
	}
	title := "Initialized Brevity project"
	if payload.Repair {
		title = "Repaired Brevity project"
	}
	renderStatusLine(stdout, title, result.Success)
	fmt.Fprintf(stdout, "project: %s\nrepo: %s\ndevRoot: %s\nvault: %s\n", payload.ProjectName, payload.RepoRoot, payload.DevRoot, payload.VaultPath)
	for _, item := range payload.Items {
		fmt.Fprintf(stdout, "%s: %s\n", item.Status, item.Path)
	}
	for _, field := range payload.RepairFields {
		fmt.Fprintf(stdout, "field %s: %s\n", field.Status, field.Name)
	}
	renderMessages(stdout, result)
	return nil
}

func initError(code string, message string, details map[string]any) (contracts.CommandResult, error) {
	raw, _ := json.Marshal(contracts.InitPayload{NoProviderExecution: true, NoWorkerExecution: true})
	result := contracts.CommandResult{Schema: contracts.CommandResultSchema, Command: initCommand, Success: false, Severity: "error", Warnings: []contracts.ResultMessage{}, Errors: []contracts.ResultMessage{{Code: code, Message: message, Details: details}}, SuggestedNextActions: []string{}, Payload: raw}
	return result, errors.New(message)
}
