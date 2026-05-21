package locking

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Clock func() time.Time

type Options struct {
	Timeout  time.Duration
	Interval time.Duration
	StaleAge time.Duration
	Now      Clock
}

type Lock struct {
	path string
	file *os.File
}

func Acquire(path string, options Options) (*Lock, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("lock path is required")
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	interval := options.Interval
	if interval <= 0 {
		interval = 50 * time.Millisecond
	}
	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}

	deadline := now().Add(timeout)
	for {
		lock, err := tryAcquire(path, now)
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if options.StaleAge > 0 && isStale(path, now(), options.StaleAge) {
			_ = os.Remove(path)
			continue
		}
		if !now().Before(deadline) {
			return nil, fmt.Errorf("state lock timeout: %s", path)
		}
		time.Sleep(interval)
	}
}

func tryAcquire(path string, now Clock) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return nil, err
	}
	lock := &Lock{path: path, file: file}
	content := fmt.Sprintf("pid=%d\ncreatedAt=%s\n", os.Getpid(), now().UTC().Format(time.RFC3339Nano))
	if _, err := file.WriteString(content); err != nil {
		_ = lock.Release()
		return nil, fmt.Errorf("write lock file: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = lock.Release()
		return nil, fmt.Errorf("flush lock file: %w", err)
	}
	return lock, nil
}

func (lock *Lock) Release() error {
	if lock == nil {
		return nil
	}
	var closeErr error
	if lock.file != nil {
		closeErr = lock.file.Close()
		lock.file = nil
	}
	removeErr := os.Remove(lock.path)
	if errors.Is(removeErr, os.ErrNotExist) {
		removeErr = nil
	}
	if closeErr != nil {
		return closeErr
	}
	return removeErr
}

func isStale(path string, now time.Time, staleAge time.Duration) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	fields := parseLockFields(string(data))
	createdAt, err := time.Parse(time.RFC3339Nano, fields["createdAt"])
	if err != nil {
		info, statErr := os.Stat(path)
		if statErr != nil {
			return false
		}
		createdAt = info.ModTime()
	}
	if now.Sub(createdAt) <= staleAge {
		return false
	}
	if pid, err := strconv.Atoi(fields["pid"]); err == nil && pid == os.Getpid() {
		return false
	}
	return true
}

func parseLockFields(content string) map[string]string {
	fields := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		fields[key] = value
	}
	return fields
}
