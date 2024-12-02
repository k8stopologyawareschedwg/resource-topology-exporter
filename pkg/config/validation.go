/*
Copyright 2022 The Kubernetes Authors.

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

package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	metricssrv "github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/metrics/server"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcemonitor"
)

func Validate(pArgs *ProgArgs) error {
	var err error

	pArgs.RTE.MetricsMode, err = metricssrv.ServingModeIsSupported(pArgs.RTE.MetricsMode)
	if err != nil {
		return err
	}

	pArgs.Resourcemonitor.PodSetFingerprintMethod, err = resourcemonitor.PFPMethodIsSupported(pArgs.Resourcemonitor.PodSetFingerprintMethod)
	if err != nil {
		return err
	}

	return nil
}

func ReadConfigletDir(configPath string) ([]fs.DirEntry, error) {
	// to make gosec happy, the validation logic must be in the same function on which we call `os.ReadFile`.
	// IOW, it seems the linter cannot track variable sanitization across functions.
	configRoot := filepath.Clean(configPath)
	cfgRoot, err := filepath.EvalSymlinks(configRoot)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// reset to original value, it somehow passed the symlink check
			cfgRoot = configRoot
		} else {
			return nil, fmt.Errorf("failed to validate config path: %w", err)
		}
	}
	// else either success or checking a non-existing path. Which can be still OK.

	pattern, err := IsConfigRootAllowed(cfgRoot, UserRunDir, UserHomeDir)
	if err != nil {
		return nil, err
	}
	if pattern == "" {
		return nil, fmt.Errorf("failed to validate configRoot path %q: does not match any allowed pattern", cfgRoot)
	}
	return os.ReadDir(cfgRoot)
}

func ReadConfiglet(configPath string) ([]byte, error) {
	// to make gosec happy, the validation logic must be in the same function on which we call `os.ReadFile`.
	// IOW, it seems the linter cannot track variable sanitization across functions.
	configRoot := filepath.Clean(configPath)
	cfgRoot, err := filepath.EvalSymlinks(configRoot)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// reset to original value, it somehow passed the symlink check
			cfgRoot = configRoot
		} else {
			return nil, fmt.Errorf("failed to validate config path: %w", err)
		}
	}
	// else either success or checking a non-existing path. Which can be still OK.

	pattern, err := IsConfigRootAllowed(cfgRoot, UserRunDir, UserHomeDir)
	if err != nil {
		return nil, err
	}
	if pattern == "" {
		return nil, fmt.Errorf("failed to validate configRoot path %q: does not match any allowed pattern", cfgRoot)
	}

	return os.ReadFile(cfgRoot)
}

// IsConfigRootAllowed checks if an *already cleaned and canonicalized* path is among the allowed list.
// use `addDirFns` to inject user-dependent paths (e.g. $HOME). Returns the matched pattern, if any,
// and error describing the failure. The error is only relevant if failed to inject user-provided paths.
func IsConfigRootAllowed(cfgPath string, addDirFns ...func() (string, error)) (string, error) {
	allowedPatterns := []string{
		"/etc/rte",
		"/run/rte",
		"/var/rte",
		"/usr/local/etc/rte",
		"/etc/resource-topology-exporter", // legacy, but still supported
	}
	for _, addDirFn := range addDirFns {
		userDir, err := addDirFn()
		if errors.Is(err, SkipDirectory) {
			continue
		}
		if err != nil {
			return "", err
		}
		allowedPatterns = append(allowedPatterns, userDir)
	}

	for _, pattern := range allowedPatterns {
		if strings.HasPrefix(cfgPath, pattern) {
			return pattern, nil
		}
	}
	return "", nil
}
