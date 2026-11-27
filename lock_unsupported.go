//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly)

package batchout

import (
	"errors"
	"time"
)

type taskLock struct{}

type lockOwner struct {
	TaskID    string
	AttemptID string
	PID       int
	StartedAt time.Time
}

func tryLockFile(string, lockOwner) (*taskLock, bool, error) {
	return nil, false, errors.New("batchout: task locks are unsupported on this platform")
}

func (l *taskLock) Close() error { return nil }
