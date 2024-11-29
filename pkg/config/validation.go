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
	"log"
	"path/filepath"
	"slices"
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

// path is constructed after validation, no need to check if it is allowed
func validateConfigletPath(configletDir, configletPath string) (string, error) {
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
	if !strings.HasPrefix(cfgRoot, configletDir) {
		return "", fmt.Errorf("resolved configlet path %q goes outside configlet dir %q", cfgRoot, configletPath)
	}
	return cfgRoot, nil
}

func validateConfigRootPath(configRoot string) (string, error) {
	if configRoot == "" {
		return "", fmt.Errorf("configRoot is not allowed to be an empty string")
	}

	cfgRoot, err := validateConfigletPath("/", configRoot)
	if err != nil {
		return "", err
	}

	allowedPatterns, err := GetAllowedConfigRoots(UserRunDir, UserHomeDir)
	if err != nil {
		return "", err
	}
	idx := slices.Index(allowedPatterns, cfgRoot)
	if idx == -1 {
		return "", fmt.Errorf("config path %q: does not match any allowed pattern (%s)", cfgRoot, strings.Join(allowedPatterns, ","))
	}
	log.Printf("configRoot %q", cfgRoot)
	return allowedPatterns[idx], nil
}

func GetAllowedConfigRoots(addDirFns ...func() (string, error)) ([]string, error) {
	allowedPatterns := []string{
		"/etc/rte",
		"/run/rte",
		"/var/rte",
		"/usr/local/etc/rte",
	}
	for _, addDirFn := range addDirFns {
		userDir, err := addDirFn()
		if errors.Is(err, SkipDirectory) {
			continue
		}
		if err != nil {
			return []string{}, err
		}
		allowedPatterns = append(allowedPatterns, userDir)
	}
	return allowedPatterns, nil
}
