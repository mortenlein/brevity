package runtimeclient

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type Client interface {
	RuntimeStateJSON() ([]byte, error)
	ProviderSetJSON(provider string, status string) ([]byte, error)
	ProviderResetJSON(provider string) ([]byte, error)
	TaskContextRefreshJSON(slug string) ([]byte, error)
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

func (client PowerShellClient) ProviderSetJSON(provider string, status string) ([]byte, error) {
	return client.runJSON("provider set "+provider+" "+status+" --json", "provider", "set", provider, status, "--json")
}

func (client PowerShellClient) ProviderResetJSON(provider string) ([]byte, error) {
	return client.runJSON("provider reset "+provider+" --json", "provider", "reset", provider, "--json")
}

func (client PowerShellClient) TaskContextRefreshJSON(slug string) ([]byte, error) {
	return client.runJSON("task context refresh "+slug+" --json", "task", "context", "refresh", slug, "--json")
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
