package notification

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

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

type Event struct {
	Timestamp     time.Time
	TimerInterval time.Duration
}

func (ev Event) IsTimer() bool {
	return ev.TimerInterval > 0
}

type EventSource interface {
	Events() <-chan Event
	Close()
	Wait()
	Stop()
	Run()
}
type UnlimitedEventSource struct {
	sleepInterval time.Duration
	filters       []FilterEvent
	watcher       *fsnotify.Watcher
	eventChan     chan Event
	stopChan      chan struct{}
	doneChan      chan struct{}
}

func NewUnlimitedEventSource() (*UnlimitedEventSource, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create the watcher: %w", err)
	}
	es := UnlimitedEventSource{
		watcher:   watcher,
		stopChan:  make(chan struct{}),
		doneChan:  make(chan struct{}),
		eventChan: make(chan Event),
	}
	return &es, nil
}

func (es *UnlimitedEventSource) Events() <-chan Event {
	return es.eventChan
}

func (es *UnlimitedEventSource) Close() {
	// for completeness sake, but will never be called
	es.watcher.Close()
}

// Wait stops the caller until the EventSource is exhausted
func (es *UnlimitedEventSource) Wait() {
	<-es.doneChan
}

func (es *UnlimitedEventSource) Stop() {
	es.stopChan <- struct{}{}
}

func (es *UnlimitedEventSource) Run() {
	es.eventChan <- Event{Timestamp: time.Now()}
	klog.V(2).Infof("initial update trigger")

	timeEvents := make(<-chan time.Time)
	if es.sleepInterval > 0 {
		ticker := time.NewTicker(es.sleepInterval)
		defer ticker.Stop()
		timeEvents = ticker.C
	}

	done := false
	for !done {
		// TODO: what about closed channels?
		select {
		case tickTs := <-timeEvents:
			es.eventChan <- Event{
				Timestamp:     tickTs,
				TimerInterval: es.sleepInterval,
			}
			klog.V(4).Infof("timer update trigger")

		case event := <-es.watcher.Events:
			klog.V(5).Infof("fsnotify event from %q: %v", event.Name, event.Op)
			if AnyFilter(es.filters, event) {
				es.eventChan <- Event{
					Timestamp: time.Now(),
				}
				klog.V(4).Infof("fsnotify update trigger")
			}

		case err := <-es.watcher.Errors:
			// and yes, keep going
			klog.Warningf("fsnotify error: %v", err)

		case <-es.stopChan:
			done = true
		}
	}
	es.doneChan <- struct{}{}
}

func (es *UnlimitedEventSource) SetInterval(interval time.Duration) error {
	if interval < 0 {
		return fmt.Errorf("interval cannot be negative: %v", interval)
	}
	if es.sleepInterval > 0 {
		return fmt.Errorf("interval already set, and only one time-based source supported")
	}
	es.sleepInterval = interval
	klog.Infof("added interval every %v", interval)
	return nil
}

func (es *UnlimitedEventSource) AddFile(notifyFilePath string) error {
	if notifyFilePath == "" {
		// nothing to do
		return nil
	}

	err := ensureNotifyFilePath(notifyFilePath)
	if err != nil {
		return err
	}

	tryToWatch(es.watcher, notifyFilePath)

	es.filters = append(es.filters, func(event fsnotify.Event) bool {
		if event.Name == notifyFilePath {
			if (event.Op & fsnotify.Chmod) == fsnotify.Chmod {
				return true
			}
			if (event.Op & fsnotify.Write) == fsnotify.Write {
				return true
			}
		}
		return false
	})
	return nil
}

// AnyFilter is a cumulative filter which returns true (hence passes)
// only ifat least one of the provided filters pass.
func AnyFilter(filters []FilterEvent, event fsnotify.Event) bool {
	for _, filter := range filters {
		if filter(event) {
			return true
		}
	}
	return false
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
