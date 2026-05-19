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
}

type PowerShellClient struct {
	ScriptPath string
}

func NewPowerShellClient() PowerShellClient {
	return PowerShellClient{ScriptPath: `.\\brevity.ps1`}
}

func (client PowerShellClient) RuntimeStateJSON() ([]byte, error) {
	scriptPath := client.ScriptPath
	if scriptPath == "" {
		scriptPath = `.\\brevity.ps1`
	}

	cmd := exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-ExecutionPolicy",
		"Bypass",
		"-File",
		scriptPath,
		"runtime",
		"state",
		"--json",
	)

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
		return nil, fmt.Errorf("brevity.ps1 runtime state --json failed: %s", message)
	}

	if len(bytes.TrimSpace(output)) == 0 {
		return nil, errors.New("PowerShell runtime state command returned empty output")
	}

	return output, nil
}
