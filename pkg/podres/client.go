/*
 * Copyright 2018 The Kubernetes Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package podres

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"k8s.io/klog/v2"
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"
)

const (
	DefaultTimeout    = 10 * time.Second
	DefaultMaxMsgSize = 1024 * 1024 * 16 // 16 MiB

	UnixProtocol = "unix"
	FakeProtocol = "fake"
)

type CleanupFunc func() error

func GetClient(endpoint string) (podresourcesapi.PodResourcesListerClient, CleanupFunc, error) {
	klog.V(4).Infof("creating a podresources client for endpoint %q", endpoint)

	var cli podresourcesapi.PodResourcesListerClient
	var cleanup CleanupFunc = nullCleanup
	var err error

	proto, path, err := ParseEndpoint(endpoint)
	if err != nil {
		return cli, cleanup, err
	}

	switch proto {
	case UnixProtocol:
		cli, cleanup, err = GetV1ClientUnix(path, DefaultTimeout, DefaultMaxMsgSize)
	case FakeProtocol:
		cli, cleanup, err = GetV1ClientFake(path)
	}

	if err != nil {
		return nil, cleanup, fmt.Errorf("failed to create podresource client: %w", err)
	}
	klog.V(4).Infof("created a podresources client for endpoint %q", endpoint)
	return cli, cleanup, nil
}

func WaitForReady(cli podresourcesapi.PodResourcesListerClient, cleanup CleanupFunc, err error) (podresourcesapi.PodResourcesListerClient, CleanupFunc, error) {
	if err != nil {
		return cli, cleanup, err
	}
	// we use List because it's the oldest endpoint and the one guaranteed to be available.
	// TODO: evaluate more lightweight option like GetAllocatableResources - we will discard
	// the return value anyway.
	_, listErr := cli.List(context.Background(), &podresourcesapi.ListPodResourcesRequest{}, grpc.WaitForReady(true))
	if listErr != nil {
		return cli, cleanup, fmt.Errorf("WaitForReady failed: %w", listErr)
	}
	return cli, cleanup, nil
}

func GetV1ClientUnix(path string, connectionTimeout time.Duration, maxMsgSize int) (podresourcesapi.PodResourcesListerClient, CleanupFunc, error) {
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, path,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(dialer),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMsgSize)),
	)
	if err != nil {
		return nil, nullCleanup, fmt.Errorf("error dialing unix endpoint at %q: %w", path, err)
	}

	cleanup := func() error { return conn.Close() }
	return podresourcesapi.NewPodResourcesListerClient(conn), cleanup, nil
}

func GetV1ClientFake(path string) (podresourcesapi.PodResourcesListerClient, CleanupFunc, error) {
	return NewFakePodResourcesLister(path), nullCleanup, nil
}

func GetV1Client(endpoint string, connectionTimeout time.Duration, maxMsgSize int) (podresourcesapi.PodResourcesListerClient, CleanupFunc, error) {
	proto, path, err := ParseEndpoint(endpoint)
	if proto != UnixProtocol {
		return nil, nullCleanup, UnsupportedProtocolError{proto: proto}
	}
	if err != nil {
		return nil, nullCleanup, err
	}
	return GetV1ClientUnix(path, connectionTimeout, maxMsgSize)
}

type UnsupportedProtocolError struct {
	proto string
}

func (e UnsupportedProtocolError) Error() string {
	return fmt.Sprintf("protocol %q not supported", e.proto)
}

func ParseEndpoint(endpoint string) (string, string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", "", err
	}
	if u.Scheme != UnixProtocol && u.Scheme != FakeProtocol {
		return "", "", UnsupportedProtocolError{proto: u.Scheme}
	}
	klog.V(6).Infof("endpoint %q -> protocol=%q path=%q", endpoint, u.Scheme, u.Path)
	return u.Scheme, u.Path, nil
}

func dialer(ctx context.Context, addr string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, UnixProtocol, addr)
}

func nullCleanup() error {
	return nil
}
