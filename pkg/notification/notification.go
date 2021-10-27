package notification

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"k8s.io/klog/v2"
)

const (
	stateCPUManager    string = "cpu_manager_state"
	stateMemoryManager string = "memory_manager_state"
	stateDeviceManager string = "kubelet_internal_checkpoint"
)

// FilterEvent returns true if the given event is relevant and should be handled
type FilterEvent func(event fsnotify.Event) bool

func AddFile(watcher *fsnotify.Watcher, notifyFilePath string) (FilterEvent, error) {
	if notifyFilePath == "" {
		// nothing to do
		return FilterEverything, nil
	}

	err := ensureNotifyFilePath(notifyFilePath)
	if err != nil {
		return FilterNothing, err
	}

	tryToWatch(watcher, notifyFilePath)

	FilterEvent := func(event fsnotify.Event) bool {
		if event.Name == notifyFilePath {
			if (event.Op & fsnotify.Chmod) == fsnotify.Chmod {
				return true
			}
			if (event.Op & fsnotify.Write) == fsnotify.Write {
				return true
			}
		}
		return false
	}
	return FilterEvent, nil
}

func AddDirs(watcher *fsnotify.Watcher, kubeletStateDirs []string) (FilterEvent, error) {
	if len(kubeletStateDirs) == 0 {
		// nothing to do
		return FilterEverything, nil
	}

	dirCount := 0
	for _, stateDir := range kubeletStateDirs {
		klog.Infof("kubelet state dir: [%s]", stateDir)
		if stateDir == "" {
			continue
		}

		tryToWatch(watcher, stateDir)
		dirCount++
	}

	if dirCount == 0 {
		// well, still legal
		klog.Infof("no valid directory to monitor given")
		return FilterEverything, nil
	}

	FilterEvent := func(event fsnotify.Event) bool {
		filename := filepath.Base(event.Name)
		if filename != stateCPUManager &&
			filename != stateMemoryManager &&
			filename != stateDeviceManager {
			return false
		}
		// turns out rename is reported as
		// 1. RENAME (old) <- unpredictable
		// 2. CREATE (new) <- we trigger here
		// admittedly we can get some false positives, but that
		// is expected to be not that big of a deal.
		return (event.Op & fsnotify.Create) == fsnotify.Create
	}
	return FilterEvent, nil
}

// MakeFilter return a cumulative filter which passes only if
// at least one of the provided filters pass.
func MakeFilter(filters ...FilterEvent) FilterEvent {
	return func(event fsnotify.Event) bool {
		for _, filter := range filters {
			if filter(event) {
				return true
			}
		}
		return false
	}
}

func FilterNothing(_ fsnotify.Event) bool {
	return false
}

func FilterEverything(_ fsnotify.Event) bool {
	return true
}

func tryToWatch(watcher *fsnotify.Watcher, fsPath string) {
	err := watcher.Add(fsPath)
	if err != nil {
		klog.Infof("error adding watch on [%s]: %v", fsPath, err)
	} else {
		klog.Infof("added watch on [%s]", fsPath)
	}
}

// ensureNotifyFilePath tries to create the notification file path in the
// filesystem. Return error if the file is not a non-zero-sized regular file.
func ensureNotifyFilePath(notifyFilePath_ string) error {
	notifyFilePath := filepath.Clean(notifyFilePath_)
	if notifyFilePath != notifyFilePath_ {
		klog.Infof("notification file path: %q -> %q", notifyFilePath_, notifyFilePath)
	}

	baseDir := filepath.Dir(notifyFilePath)
	err := os.MkdirAll(baseDir, 0755)
	if err != nil {
		klog.Infof("error creating the notify path %q: %v", baseDir, err)
		return err
	}

	if info, err := os.Stat(notifyFilePath); err == nil {
		isReg := info.Mode().IsRegular()
		if info.Size() > 0 || !isReg {
			return fmt.Errorf("cannot use %q: already exists with size=%d isRegular=%t", notifyFilePath, info.Size(), isReg)
		}
	}

	fh, err := os.Create(notifyFilePath)
	if err != nil {
		return err
	}
	return fh.Close() // how can this fail?
}
