package refresh

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// ErrBusy means another swingdesk process is refreshing the same data dir.
var ErrBusy = errors.New("another swingdesk process is refreshing")

// fileLock is an exclusive, non-blocking flock held for one refresh so two
// processes never reconcile or import each other's live runs.
type fileLock struct{ f *os.File }

func acquireLock(path string) (fileLock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fileLock{}, fmt.Errorf("open lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return fileLock{}, ErrBusy
		}
		return fileLock{}, fmt.Errorf("lock %s: %w", path, err)
	}
	return fileLock{f: f}, nil
}

// release unlocks; closing the descriptor drops the flock.
func (l fileLock) release() error {
	if l.f == nil {
		return nil
	}
	return l.f.Close()
}
