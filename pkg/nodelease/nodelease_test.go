// SPDX-License-Identifier: Apache-2.0

package nodelease

import (
	"path/filepath"
	"testing"
)

func TestTryLock_Acquires(t *testing.T) {
	leaseFile := filepath.Join(t.TempDir(), "lease")
	nl, err := New(leaseFile)
	if err != nil {
		t.Fatalf("failed to create NodeLease: %v", err)
	}
	defer nl.Close()

	if !nl.TryLock() {
		t.Fatal("expected TryLock to succeed on uncontested lease")
	}
}

func TestTryLock_Idempotent(t *testing.T) {
	leaseFile := filepath.Join(t.TempDir(), "lease")
	nl, err := New(leaseFile)
	if err != nil {
		t.Fatalf("failed to create NodeLease: %v", err)
	}
	defer nl.Close()

	if !nl.TryLock() {
		t.Fatal("first TryLock should succeed")
	}
	if !nl.TryLock() {
		t.Fatal("second TryLock should succeed (idempotent)")
	}
}

func TestTryLock_Contention(t *testing.T) {
	leaseFile := filepath.Join(t.TempDir(), "lease")
	first, err := New(leaseFile)
	if err != nil {
		t.Fatalf("failed to create first NodeLease: %v", err)
	}
	defer first.Close()

	if !first.TryLock() {
		t.Fatal("first lock should succeed")
	}

	second, err := New(leaseFile)
	if err != nil {
		t.Fatalf("failed to create second NodeLease: %v", err)
	}
	defer second.Close()

	if second.TryLock() {
		t.Fatal("second lock should fail while first holds the lease")
	}
}

func TestTryLock_ReleaseOnClose(t *testing.T) {
	leaseFile := filepath.Join(t.TempDir(), "lease")
	first, err := New(leaseFile)
	if err != nil {
		t.Fatalf("failed to create first NodeLease: %v", err)
	}

	if !first.TryLock() {
		t.Fatal("first lock should succeed")
	}

	second, err := New(leaseFile)
	if err != nil {
		t.Fatalf("failed to create second NodeLease: %v", err)
	}
	defer second.Close()

	if second.TryLock() {
		t.Fatal("second lock should fail while first is held")
	}

	if err := first.Close(); err != nil {
		t.Fatalf("failed to close first NodeLease: %v", err)
	}

	if !second.TryLock() {
		t.Fatal("second lock should succeed after first is released")
	}
}

func TestLeaseFilePath(t *testing.T) {
	tests := []struct {
		notifyPath string
		expected   string
	}{
		{"/host-run/rte/notify", "/host-run/rte/lease"},
		{"/var/run/rte/notify", "/var/run/rte/lease"},
		{"", ""},
	}
	for _, tc := range tests {
		got := LeaseFilePath(tc.notifyPath)
		if got != tc.expected {
			t.Errorf("LeaseFilePath(%q) = %q, want %q", tc.notifyPath, got, tc.expected)
		}
	}
}
