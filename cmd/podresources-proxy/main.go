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

package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"k8s.io/klog/v2"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/proxy"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/version"
)

const (
	programName = "podresources-proxy"

	defaultKubeletSocketPath = "/var/lib/kubelet/pod-resources/kubelet.sock"
	defaultListenSocketPath  = "/run/rte/podresources.sock"
)

type progArgs struct {
	Verbose           int
	KubeletSocketPath string
	ListenSocketPath  string
}

func setDefaults(pa *progArgs) {
	pa.KubeletSocketPath = defaultKubeletSocketPath
	pa.ListenSocketPath = defaultListenSocketPath
}

func parseFlags(pa *progArgs, args ...string) error {
	flags := flag.NewFlagSet(programName, flag.ExitOnError)

	klog.InitFlags(flags)

	flags.StringVar(&pa.KubeletSocketPath, "kubelet-socket", pa.KubeletSocketPath, "upstream kubelet podresources socket path.")
	flags.StringVar(&pa.ListenSocketPath, "listen-socket", pa.ListenSocketPath, "downstream listen podresources socket path.")

	return flags.Parse(args)
}

func main() {
	klog.InfoS("starting", "program", programName, "version", version.Get())
	defer klog.InfoS("stopped", "program", programName, "version", version.Get())

	var pArgs progArgs
	setDefaults(&pArgs)
	err := parseFlags(&pArgs, os.Args[1:]...)

	if err != nil {
		klog.Fatalf("failed to parse args: %v", err)
	}

	klog.V(2).InfoS("connection parameters", "kubelet", pArgs.KubeletSocketPath, "listen", pArgs.ListenSocketPath)

	cli, cleanup, err := podres.GetClient("unix://" + pArgs.KubeletSocketPath) // TODO: do we need smarter logic?
	if err != nil {
		klog.Fatalf("failed to create a podresources client: %v", err)
	}

	inst := proxy.New(cli, cleanup)
	err = inst.Setup("unix://" + pArgs.ListenSocketPath) // TODO: do we need smarter logic?
	if err != nil {
		klog.Fatalf("failed to setupthe proxied podresources API: %v", err)
	}

	go inst.Serve()
	klog.V(4).InfoS("running...")

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	s := <-sigs

	log.Printf("received %s", s)
}
