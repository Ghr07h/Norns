//go:build darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd

package workspace

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func openLifecycleFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
}

func retryLifecycleFileError(error) bool { return false }

func removeLifecycleFile(_ *os.File, path string) error { return os.Remove(path) }

func tryLifecycleFileLock(file *os.File, shared bool) (bool, error) {
	operation := unix.LOCK_EX | unix.LOCK_NB
	if shared {
		operation = unix.LOCK_SH | unix.LOCK_NB
	}
	err := unix.Flock(int(file.Fd()), operation)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) {
		return false, nil
	}
	return err == nil, err
}

func unlockLifecycleFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
