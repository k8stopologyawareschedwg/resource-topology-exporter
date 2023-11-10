/*
Copyright 2023 The Kubernetes Authors.

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

package remoteexec

import (
	"bytes"
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"

	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// CommandOnPod runs command in the pod and returns buffer output
func CommandOnPod(ctx context.Context, c kubernetes.Interface, pod *corev1.Pod, cntName string, command ...string) ([]byte, []byte, error) {
	return CommandOnPodByNames(ctx, c, pod.Namespace, pod.Name, cntName, command...)
}

func CommandOnPodByNames(ctx context.Context, c kubernetes.Interface, podNamespace, podName, cntName string, command ...string) ([]byte, []byte, error) {
	var outputBuf bytes.Buffer
	var errorBuf bytes.Buffer

	req := c.CoreV1().RESTClient().
		Post().
		Namespace(podNamespace).
		Resource("pods").
		Name(podName).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: cntName,
			Command:   command,
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       true,
		}, scheme.ParameterCodec)

	cfg, err := config.GetConfig()
	if err != nil {
		return nil, nil, err
	}

	exec, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
	if err != nil {
		return nil, nil, err
	}

	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  os.Stdin,
		Stdout: &outputBuf,
		Stderr: &errorBuf,
		Tty:    true,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to run command %v: %w", command, err)
	}

	return outputBuf.Bytes(), errorBuf.Bytes(), nil
}
