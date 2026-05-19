package runtimeclient

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/mortenlein/brevity/internal/commands"
)

type Client interface {
	RuntimeStateJSON() ([]byte, error)
	DoctorJSON() ([]byte, error)
	ProviderSetJSON(provider string, status string) ([]byte, error)
	ProviderResetJSON(provider string) ([]byte, error)
	TaskContextRefreshJSON(slug string) ([]byte, error)
	TaskCleanupJSON(slug string) ([]byte, error)
	TaskNewJSON(slug string) ([]byte, error)
	TaskRunJSON(slug string, profile string, smoke bool) ([]byte, error)
	TaskRuntimeInfoJSON(slug string) ([]byte, error)
	TaskRunsJSON(slug string) ([]byte, error)
	TaskRunsReconcileJSON() ([]byte, error)
	TaskRunsRetentionJSON() ([]byte, error)
	TaskRunsCompactJSON() ([]byte, error)
}

type PowerShellClient struct {
	ScriptPath string
}

func NewPowerShellClient() PowerShellClient {
	return PowerShellClient{ScriptPath: `.\\brevity.ps1`}
}

func (client PowerShellClient) RuntimeStateJSON() ([]byte, error) {
	return client.runJSON("runtime state --json", "runtime", "state", "--json")
}

func (client PowerShellClient) DoctorJSON() ([]byte, error) {
	return client.runJSON(jsonDescription(commands.Doctor), commands.Doctor.JSONArgs()...)
}

func (client PowerShellClient) ProviderSetJSON(provider string, status string) ([]byte, error) {
	return client.runJSON(jsonDescription(commands.ProviderSet, provider, status), commands.ProviderSet.JSONArgs(provider, status)...)
}

func (client PowerShellClient) ProviderResetJSON(provider string) ([]byte, error) {
	return client.runJSON(jsonDescription(commands.ProviderReset, provider), commands.ProviderReset.JSONArgs(provider)...)
}

func (client PowerShellClient) TaskContextRefreshJSON(slug string) ([]byte, error) {
	return client.runJSON(jsonDescription(commands.TaskContextRefresh, slug), commands.TaskContextRefresh.JSONArgs(slug)...)
}

func (client PowerShellClient) TaskCleanupJSON(slug string) ([]byte, error) {
	return client.runJSON(jsonDescription(commands.TaskCleanup, slug, "--force"), commands.TaskCleanup.JSONArgs(slug, "--force")...)
}

func (client PowerShellClient) TaskNewJSON(slug string) ([]byte, error) {
	return client.runJSON(jsonDescription(commands.TaskNew, slug), commands.TaskNew.JSONArgs(slug)...)
}

func (client PowerShellClient) TaskRunJSON(slug string, profile string, smoke bool) ([]byte, error) {
	description := jsonDescription(commands.TaskRun, slug, "--execute")
	args := commands.TaskRun.JSONArgs(slug, "--execute")
	if profile != "" {
		description += " --profile " + profile
		args = append(args, "--profile", profile)
	}
	if smoke {
		description += " --smoke"
		args = append(args, "--smoke")
	}
	return client.runJSON(description, args...)
}

func (client PowerShellClient) TaskRuntimeInfoJSON(slug string) ([]byte, error) {
	return client.runJSON(jsonDescription(commands.TaskRuntimeInfo, slug), commands.TaskRuntimeInfo.JSONArgs(slug)...)
}

func (client PowerShellClient) TaskRunsJSON(slug string) ([]byte, error) {
	return client.runJSON(jsonDescription(commands.TaskRuns, slug), commands.TaskRuns.JSONArgs(slug)...)
}

func (client PowerShellClient) TaskRunsReconcileJSON() ([]byte, error) {
	return client.runJSON(jsonDescription(commands.TaskRunsReconcile, "--dry-run"), commands.TaskRunsReconcile.JSONArgs("--dry-run")...)
}

func (client PowerShellClient) TaskRunsRetentionJSON() ([]byte, error) {
	return client.runJSON(jsonDescription(commands.TaskRunsRetention, "--dry-run"), commands.TaskRunsRetention.JSONArgs("--dry-run")...)
}

func (client PowerShellClient) TaskRunsCompactJSON() ([]byte, error) {
	return client.runJSON(jsonDescription(commands.TaskRunsCompact, "--dry-run"), commands.TaskRunsCompact.JSONArgs("--dry-run")...)
}

func jsonDescription(command commands.Command, extra ...string) string {
	parts := append([]string{command.Name()}, extra...)
	parts = append(parts, "--json")
	return strings.Join(parts, " ")
}

func (client PowerShellClient) runJSON(description string, args ...string) ([]byte, error) {
	scriptPath := client.ScriptPath
	if scriptPath == "" {
		scriptPath = `.\\brevity.ps1`
	}

	commandArgs := []string{
		"-NoProfile",
		"-ExecutionPolicy",
		"Bypass",
		"-File",
		scriptPath,
	}
	commandArgs = append(commandArgs, args...)

	cmd := exec.Command("powershell.exe", commandArgs...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, errors.New("PowerShell command not found: install PowerShell or ensure powershell.exe is on PATH")
		}

		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		if len(bytes.TrimSpace(output)) > 0 {
			return output, fmt.Errorf("brevity.ps1 %s failed: %s", description, message)
		}
		return nil, fmt.Errorf("brevity.ps1 %s failed: %s", description, message)
	}

	if len(bytes.TrimSpace(output)) == 0 {
		return nil, fmt.Errorf("PowerShell %s command returned empty output", description)
	}

	return output, nil
}
