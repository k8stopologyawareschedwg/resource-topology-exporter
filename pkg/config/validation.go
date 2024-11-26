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

func validateConfigPath(configletPath string) (string, error) {
	configRoot := filepath.Clean(configletPath)
	cfgRoot, err := filepath.EvalSymlinks(configRoot)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// reset to original value, it somehow passed the symlink check
			cfgRoot = configRoot
		} else {
			return "", fmt.Errorf("failed to validate configRoot path: %w", err)
		}
	}
	// else either success or checking a non-existing path. Which can be still OK.

	pattern, err := IsConfigRootAllowed(cfgRoot, UserRunDir, UserHomeDir)
	if err != nil {
		return "", err
	}
	if pattern == "" {
		return "", fmt.Errorf("failed to validate configRoot path %q: does not match any allowed pattern", cfgRoot)
	}
	return cfgRoot, nil

}

func validateConfigRootPath(configRoot string) (string, error) {
	if configRoot == "" {
		return "", fmt.Errorf("configRoot is not allowed to be an empty string")
	}
	return validateConfigPath(configRoot)
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
