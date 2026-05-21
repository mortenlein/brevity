package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const DirectoryName = ".brevity"

type Store struct {
	RepoRoot string
}

func NewStore(repoRoot string) (Store, error) {
	if repoRoot == "" {
		wd, err := os.Getwd()
		if err != nil {
			return Store{}, fmt.Errorf("get working directory: %w", err)
		}
		repoRoot = wd
	}
	return Store{RepoRoot: repoRoot}, nil
}

func (store Store) BrevityRoot() string {
	return filepath.Join(store.RepoRoot, DirectoryName)
}

func (store Store) Path(name string) string {
	return filepath.Join(store.BrevityRoot(), name)
}

func (store Store) LockPath() string {
	return store.Path("state.lock")
}

func (store Store) ReadJSON(name string, target any) (bool, error) {
	data, err := os.ReadFile(store.Path(name))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, fmt.Errorf("read %s: %w", name, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return false, fmt.Errorf("read %s: file is empty", name)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return false, fmt.Errorf("parse %s: %w", name, err)
	}
	return false, nil
}

func (store Store) WriteJSON(name string, value any) error {
	if err := os.MkdirAll(store.BrevityRoot(), 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", name, err)
	}
	data = append(data, '\n')
	path := store.Path(name)
	temp, err := os.CreateTemp(store.BrevityRoot(), name+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp %s: %w", name, err)
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temp %s: %w", name, err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("flush temp %s: %w", name, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temp %s: %w", name, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", name, err)
	}
	return nil
}
