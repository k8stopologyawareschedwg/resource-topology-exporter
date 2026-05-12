// SPDX-License-Identifier: Apache-2.0

package nodelease

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"k8s.io/klog/v2"
)

const LeaseFileName = "lease"

const AutodetectLeaseFromNotify = "default"

type Lease interface {
	TryLock() bool
	Close() error
}

type flockLease struct {
	fd   int
	path string
	held bool
}

type nullLease struct{}

// NewNull returns a no-op lease that always succeeds.
func NewNull() Lease {
	return &nullLease{}
}

func (hl *nullLease) TryLock() bool { return true }
func (hl *nullLease) Close() error  { return nil }

// New opens (or creates) the lease file at the given path.
// The lock is NOT acquired until TryLock is called.
func New(leaseFilePath string) (Lease, error) {
	dir := filepath.Dir(leaseFilePath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("cannot create lease directory %q: %w", dir, err)
	}

	fd, err := syscall.Open(leaseFilePath, syscall.O_CREAT|syscall.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("cannot open lease file %q: %w", leaseFilePath, err)
	}
	klog.Infof("nodelease: opened lease file %q (fd=%d)", leaseFilePath, fd)
	return &flockLease{
		fd:   fd,
		path: leaseFilePath,
	}, nil
}

// TryLock attempts to acquire the exclusive flock. If the lease is already
// held by this instance, returns true immediately. If another process holds
// the lock, returns false without blocking.
func (nl *flockLease) TryLock() bool {
	if nl.held {
		return true
	}
	err := syscall.Flock(nl.fd, syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		klog.V(4).Infof("nodelease: lease not acquired on %q: %v", nl.path, err)
		return false
	}
	nl.held = true
	klog.Infof("nodelease: acquired lease on %q", nl.path)
	return true
}

// Close releases the flock (if held) by closing the file descriptor.
func (nl *flockLease) Close() error {
	err := syscall.Close(nl.fd)
	if err != nil {
		return fmt.Errorf("cannot close lease file %q: %w", nl.path, err)
	}
	nl.held = false
	klog.Infof("nodelease: released lease on %q", nl.path)
	return nil
}

// LeaseFilePath returns the default lease file path derived from the
// notify file path. If notifyFilePath is empty, returns empty string.
func LeaseFilePath(notifyFilePath string) string {
	if notifyFilePath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(notifyFilePath), LeaseFileName)
}
