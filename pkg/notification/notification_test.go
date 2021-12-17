package notification

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
)

func TestAnyFilter(t *testing.T) {
	type testCase struct {
		name     string
		filters  []FilterEvent
		event    fsnotify.Event
		expected bool
	}

	testCases := []testCase{
		{
			name:     "nothing, nothing",
			filters:  []FilterEvent{FilterNothing, FilterNothing},
			expected: false,
		},
		{
			name:     "everything, nothing",
			filters:  []FilterEvent{FilterEverything, FilterNothing},
			expected: true,
		},
		{
			name:     "nothing, everything",
			filters:  []FilterEvent{FilterNothing, FilterEverything},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := AnyFilter(tc.filters, tc.event)
			if tc.expected != got {
				t.Fatalf("%s event=%v expected=%t got=%t", tc.name, tc.event, tc.expected, got)
			}
		})
	}
}

// TODO: we should not test private functions. But exporting functions for the sake of enabling unit-test is not
// a clearly better approach, so at least we keep it private.

func TestEnsureNotifyFilePath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testnotif")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	defer os.RemoveAll(tmpDir)

	testPath := filepath.Join(tmpDir, "nested", "notify")

	err = ensureNotifyFilePath(testPath)
	if err != nil {
		t.Fatalf("unexpected failure: %v", err)
	}

	err = ensureNotifyFilePath(testPath)
	if err != nil {
		t.Fatalf("unexpected failure: %v", err)
	}
}

func TestEnsureNotifyFilePathExistingWithUNSAFEContent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testnotif")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	defer os.RemoveAll(tmpDir)

	testPath := filepath.Join(tmpDir, "nested", "notify")

	err = ensureNotifyFilePath(testPath)
	if err != nil {
		t.Fatalf("unexpected failure: %v", err)
	}

	err = os.WriteFile(testPath, []byte("the file is no longer empty"), 0644)
	if err != nil {
		t.Fatalf("error setting test data content: %v", err)
	}

	err = ensureNotifyFilePath(testPath)
	if err == nil {
		t.Fatalf("unexpected SUCCESS: %v", err)
	}
}

func TestEnsureNotifyFilePathExistingSymlink(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "testnotif")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	defer os.RemoveAll(tmpDir)

	err = os.MkdirAll(filepath.Join(tmpDir, "nested"), 0755)
	if err != nil {
		t.Fatalf("error creating the test directory tree: %v", err)
	}

	canaryPath := filepath.Join(tmpDir, "nested", "canary")
	err = os.WriteFile(canaryPath, []byte("superimportant data which should not be changed"), 0644)
	if err != nil {
		t.Fatalf("error setting canary content: %v", err)
	}
	testPath := filepath.Join(tmpDir, "nested", "notify")
	err = os.Symlink(canaryPath, testPath)
	if err != nil {
		t.Fatalf("error setting symlink to canary: %v", err)
	}

	err = ensureNotifyFilePath(testPath)
	if err == nil {
		t.Fatalf("unexpected SUCCESS: %v", err)
	}
}
