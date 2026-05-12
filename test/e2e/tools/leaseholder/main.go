package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/nodelease"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <lease-file-path>\n", os.Args[0])
		os.Exit(1)
	}
	nl, err := nodelease.New(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create lease: %v\n", err)
		os.Exit(1)
	}
	if !nl.TryLock() {
		fmt.Fprintf(os.Stderr, "failed to acquire lease\n")
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "lease acquired, holding until killed\n")
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}
