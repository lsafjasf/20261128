//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package batchout

import (
	"encoding/json"
	"errors"
	"os"
	"syscall"
	"time"
)

type taskLock struct {
	file *os.File
}

type lockOwner struct {
	TaskID    string    `json:"task_id"`
	AttemptID string    `json:"attempt_id,omitempty"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
}

func tryLockFile(path string, owner lockOwner) (*taskLock, bool, error) {
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		return nil, false, err
	}
	f := os.NewFile(uintptr(fd), path)
	if f == nil {
		_ = syscall.Close(fd)
		return nil, false, errors.New("batchout: create lock file handle")
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, false, nil
		}
		return nil, false, err
	}
	data, err := json.Marshal(owner)
	if err == nil {
		data = append(data, '\n')
		err = f.Truncate(0)
	}
	if err == nil {
		_, err = f.Seek(0, 0)
	}
	if err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	if err != nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
		return nil, false, err
	}
	return &taskLock{file: f}, true, nil
}

func (l *taskLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	return errors.Join(unlockErr, closeErr)
}
