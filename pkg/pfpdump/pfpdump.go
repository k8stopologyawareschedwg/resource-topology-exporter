/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pfpdump

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"k8s.io/klog/v2"

	"github.com/k8stopologyawareschedwg/podfingerprint"
)

type Handle struct {
	Dumpfile string
}

func Execute(hnd Handle, ctx context.Context) {
	if hnd.Dumpfile == "" {
		klog.Infof("pfpdump: no status file, nothing to do")
		return
	}

	ch := make(chan podfingerprint.Status)
	podfingerprint.SetCompletionSink(ch)
	go dumpLoop(ctx, hnd, ch)

	klog.Infof("pfpdump: dumping loop running with sink=%q", hnd.Dumpfile)
}

func dumpLoop(ctx context.Context, hnd Handle, updates <-chan podfingerprint.Status) {
	klog.V(4).Infof("dump loop started")
	defer klog.V(4).Infof("dump loop finished")

	dir, file := filepath.Split(hnd.Dumpfile)
	for {
		select {
		case <-ctx.Done():
			return
		case st := <-updates:
			err := ToFile(st, dir, file)
			klog.V(6).Infof("pfpdump: executed fullPath=%q statusFile=%q err=%v", hnd.Dumpfile, file, err)
			// intentionally ignore error, we must keep going.
		}
	}
}

func ToFile(st podfingerprint.Status, dir, file string) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}

	dst, err := os.CreateTemp(dir, "__"+file)
	if err != nil {
		return err
	}
	defer os.Remove(dst.Name()) // either way, we need to get rid of this

	_, err = dst.Write(data)
	if err != nil {
		return err
	}

	err = dst.Close()
	if err != nil {
		return err
	}

	return os.Rename(dst.Name(), filepath.Join(dir, file))
}
