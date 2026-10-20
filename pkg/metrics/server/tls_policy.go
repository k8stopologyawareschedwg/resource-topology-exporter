/*
Copyright The Kubernetes Authors.

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

package metrics

import (
	"crypto/tls"
	"fmt"
	"strconv"
	"strings"

	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/klog/v2"
)

// metricsTLSPolicyOpts returns TLS option funcs that apply MinVersion, MaxVersion,
// CipherSuites, and CurvePreferences when minTLSVersion, cipherSuites, and/or curvePreferences are set.
// It does not set application-layer protocol (ALPN); callers composing a full metrics listener should add that separately.
// Empty minTLSVersion, cipherSuites, and curvePreferences means no policy is applied (callers should not append these opts).
// Version names follow Kubernetes apiserver/scheduler flags (e.g. VersionTLS12, VersionTLS13).
// Cipher names follow crypto/tls cipher suite names as accepted by k8s.io/component-base/cli/flag.TLSCipherSuites
// (comma-separated). Curve preferences are comma-separated numeric tls.CurveID values.
func metricsTLSPolicyOpts(minTLSVersion, cipherSuitesCSV, curvePreferenceStr string) ([]func(*tls.Config), error) {
	minName := strings.TrimSpace(minTLSVersion)
	cipherNames := splitNonEmptyCSV(cipherSuitesCSV)
	curveIDs, err := parseCurveIDs(curvePreferenceStr)
	if err != nil {
		return nil, err
	}

	if minName == "" && len(cipherNames) == 0 && len(curveIDs) == 0 {
		return nil, nil
	}

	minVer, err := cliflag.TLSVersion(minName)
	if err != nil {
		return nil, err
	}
	cipherIDsUint16, err := cliflag.TLSCipherSuites(cipherNames)
	if err != nil {
		return nil, err
	}

	if minVer == tls.VersionTLS13 {
		return []func(*tls.Config){tls13PolicyOpt(cipherNames, curveIDs)}, nil
	}
	return []func(*tls.Config){tls12AndEarlierPolicyOpt(minVer, cipherIDsUint16, curveIDs)}, nil
}

func tls13PolicyOpt(ignoredCipherNames []string, curveIDs []tls.CurveID) func(*tls.Config) {
	if len(ignoredCipherNames) > 0 {
		klog.V(2).InfoS("TLS 1.3 uses fixed cipher suites; ignoring configured TLS cipher list",
			"cipherSuiteCount", len(ignoredCipherNames))
	}
	return func(cfg *tls.Config) {
		cfg.MinVersion = tls.VersionTLS13
		cfg.MaxVersion = tls.VersionTLS13
		cfg.CipherSuites = nil
		if len(curveIDs) > 0 {
			cfg.CurvePreferences = curveIDs
		}
	}
}

func tls12AndEarlierPolicyOpt(minVer uint16, cipherIDs []uint16, curveIDs []tls.CurveID) func(*tls.Config) {
	return func(cfg *tls.Config) {
		cfg.MinVersion = minVer
		cfg.CipherSuites = cipherIDs
		if len(curveIDs) > 0 {
			cfg.CurvePreferences = curveIDs
		}
	}
}

func parseCurveIDs(curvePreferenceStr string) ([]tls.CurveID, error) {
	curveStrs := splitNonEmptyCSV(curvePreferenceStr)
	if len(curveStrs) == 0 {
		return nil, nil
	}

	curveIDs := make([]tls.CurveID, 0, len(curveStrs))
	for _, curveStr := range curveStrs {
		curveID, err := strconv.ParseUint(curveStr, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid curve ID %q: %w", curveStr, err)
		}
		curveIDs = append(curveIDs, tls.CurveID(curveID))
	}
	return curveIDs, nil
}

func splitNonEmptyCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// withHTTP11 sets ALPN for the metrics HTTP/1.1 listener.
func withHTTP11() func(*tls.Config) {
	return func(cfg *tls.Config) {
		cfg.NextProtos = []string{"http/1.1"}
	}
}

// buildSecureMetricsTLSOpts composes TLSOpts for HTTPS metrics: HTTP/1.1 ALPN, optional
// TLS version/cipher/curve policy, and client certificate settings.
func buildSecureMetricsTLSOpts(tlsConf TLSConfig) ([]func(*tls.Config), error) {
	policyOpts, err := metricsTLSPolicyOpts(tlsConf.MinTLSVersion, tlsConf.CipherSuites, tlsConf.CurvePreferences)
	if err != nil {
		return nil, fmt.Errorf("metrics TLS policy: %w", err)
	}
	out := []func(*tls.Config){withHTTP11()}
	out = append(out, policyOpts...)
	out = append(out, WithClientAuth(tlsConf.WantCliAuth))
	return out, nil
}
