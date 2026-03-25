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
	"reflect"
	"testing"
)

func TestMetricsTLSPolicyOpts(t *testing.T) {
	type testCase struct {
		name          string
		minTLSVersion string
		cipherSuites  string
		expectErr     bool
		wantOptCount  int
		check         func(t *testing.T, cfg *tls.Config)
	}

	testCases := []testCase{
		{
			name:         "empty min and empty ciphers returns no opts",
			wantOptCount: 0,
		},
		{
			name:          "TLS 1.2 with two ciphers",
			minTLSVersion: "VersionTLS12",
			cipherSuites:  "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
			wantOptCount:  1,
			check: func(t *testing.T, cfg *tls.Config) {
				t.Helper()
				checkMinTLSVersion(t, cfg, tls.VersionTLS12)
				if cfg.MaxVersion != 0 {
					t.Errorf("MaxVersion should be unset, got %x", cfg.MaxVersion)
				}
				if len(cfg.CipherSuites) != 2 {
					t.Errorf("CipherSuites: want 2 entries, got %d", len(cfg.CipherSuites))
				}
			},
		},
		{
			name:          "TLS 1.2 min only uses Go default ciphers",
			minTLSVersion: "VersionTLS12",
			wantOptCount:  1,
			check: func(t *testing.T, cfg *tls.Config) {
				t.Helper()
				checkMinTLSVersion(t, cfg, tls.VersionTLS12)
				if len(cfg.CipherSuites) != 0 {
					t.Errorf("CipherSuites: want empty when unset, got %#v", cfg.CipherSuites)
				}
			},
		},
		{
			name:         "ciphers only defaults min to TLS 1.2",
			cipherSuites: "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
			wantOptCount: 1,
			check: func(t *testing.T, cfg *tls.Config) {
				t.Helper()
				checkMinTLSVersion(t, cfg, tls.VersionTLS12)
				if len(cfg.CipherSuites) != 1 {
					t.Errorf("CipherSuites: got %#v", cfg.CipherSuites)
				}
			},
		},
		{
			name:          "TLS 1.3 pins min/max and clears explicit cipher list",
			minTLSVersion: "VersionTLS13",
			cipherSuites:  "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
			wantOptCount:  1,
			check: func(t *testing.T, cfg *tls.Config) {
				t.Helper()
				checkTLS13OnlyNoExplicitCiphers(t, cfg)
			},
		},
		{
			name:          "TLS 1.3 without cipher list",
			minTLSVersion: "VersionTLS13",
			wantOptCount:  1,
			check: func(t *testing.T, cfg *tls.Config) {
				t.Helper()
				checkTLS13OnlyNoExplicitCiphers(t, cfg)
			},
		},
		{
			name:          "TLS 1.11 min with cipher",
			minTLSVersion: "VersionTLS11",
			cipherSuites:  "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA",
			wantOptCount:  1,
			check: func(t *testing.T, cfg *tls.Config) {
				t.Helper()
				checkMinTLSVersion(t, cfg, tls.VersionTLS11)
				if len(cfg.CipherSuites) != 1 {
					t.Errorf("CipherSuites: want 1 entry, got %d", len(cfg.CipherSuites))
				}
			},
		},
		{
			name:          "unknown min version",
			minTLSVersion: "VersionTLS99",
			expectErr:     true,
		},
		{
			name:          "unknown cipher",
			minTLSVersion: "VersionTLS12",
			cipherSuites:  "NOT_A_REAL_CIPHER",
			expectErr:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts, err := metricsTLSPolicyOpts(tc.minTLSVersion, tc.cipherSuites)
			if !checkTLSPolicyOutcome(t, err, tc.expectErr, opts, tc.wantOptCount) {
				return
			}
			if tc.check != nil && len(opts) > 0 {
				cfg := &tls.Config{}
				applyTLSOptionFuncs(cfg, opts[:1])
				tc.check(t, cfg)
			}
		})
	}
}

func TestSplitNonEmptyCSV(t *testing.T) {
	type testCase struct {
		name     string
		input    string
		expected []string
	}

	testCases := []testCase{
		{
			name:     "trims and drops empties",
			input:    " a , , b,c ",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "single token",
			input:    "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
			expected: []string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "only commas and spaces",
			input:    " , ,  ",
			expected: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitNonEmptyCSV(tc.input)
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("splitNonEmptyCSV(%q): got %#v, want %#v", tc.input, got, tc.expected)
			}
		})
	}
}

func TestBuildSecureMetricsTLSOpts(t *testing.T) {
	type testCase struct {
		name         string
		tlsConf      TLSConfig
		expectErr    bool
		wantOptCount int
		check        func(t *testing.T, cfg *tls.Config)
	}

	testCases := []testCase{
		{
			name:         "no policy adds ALPN then no client auth",
			tlsConf:      TLSConfig{},
			wantOptCount: 2,
			check: func(t *testing.T, cfg *tls.Config) {
				t.Helper()
				checkNextProtosHTTP11(t, cfg)
				checkClientAuth(t, cfg, tls.NoClientCert)
			},
		},
		{
			name: "policy and client certificate required",
			tlsConf: TLSConfig{
				MinTLSVersion: "VersionTLS12",
				CipherSuites:  "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
				WantCliAuth:   true,
			},
			wantOptCount: 3,
			check: func(t *testing.T, cfg *tls.Config) {
				t.Helper()
				checkNextProtosHTTP11(t, cfg)
				checkMinTLSVersion(t, cfg, tls.VersionTLS12)
				checkClientAuth(t, cfg, tls.RequireAndVerifyClientCert)
			},
		},
		{
			name: "invalid policy surfaces error",
			tlsConf: TLSConfig{
				MinTLSVersion: "VersionTLS12",
				CipherSuites:  "BAD_CIPHER",
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts, err := buildSecureMetricsTLSOpts(tc.tlsConf)
			if !checkTLSPolicyOutcome(t, err, tc.expectErr, opts, tc.wantOptCount) {
				return
			}
			if tc.check != nil {
				cfg := &tls.Config{}
				applyTLSOptionFuncs(cfg, opts)
				tc.check(t, cfg)
			}
		})
	}
}

// checkTLSPolicyOutcome checks error expectations and option count. It returns false if the
// subtest should not continue (expected error path or fatal test failure).
func checkTLSPolicyOutcome(t *testing.T, err error, expectErr bool, opts []func(*tls.Config), wantOptCount int) bool {
	t.Helper()
	if expectErr {
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		return false
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(opts) != wantOptCount {
		t.Fatalf("want %d TLS opt funcs, got %d", wantOptCount, len(opts))
	}
	return true
}

func applyTLSOptionFuncs(cfg *tls.Config, opts []func(*tls.Config)) {
	for _, fn := range opts {
		fn(cfg)
	}
}

func checkMinTLSVersion(t *testing.T, cfg *tls.Config, want uint16) {
	t.Helper()
	if cfg.MinVersion != want {
		t.Errorf("MinVersion: got %x want %x", cfg.MinVersion, want)
	}
}

func checkNextProtosHTTP11(t *testing.T, cfg *tls.Config) {
	t.Helper()
	if len(cfg.NextProtos) != 1 || cfg.NextProtos[0] != "http/1.1" {
		t.Errorf("NextProtos: want [http/1.1], got %#v", cfg.NextProtos)
	}
}

func checkTLS13OnlyNoExplicitCiphers(t *testing.T, cfg *tls.Config) {
	t.Helper()
	if cfg.MinVersion != tls.VersionTLS13 || cfg.MaxVersion != tls.VersionTLS13 {
		t.Errorf("want TLS 1.3 only, got min=%x max=%x", cfg.MinVersion, cfg.MaxVersion)
	}
	if cfg.CipherSuites != nil {
		t.Errorf("TLS 1.3 should leave CipherSuites nil, got %#v", cfg.CipherSuites)
	}
}

func checkClientAuth(t *testing.T, cfg *tls.Config, want tls.ClientAuthType) {
	t.Helper()
	if cfg.ClientAuth != want {
		t.Errorf("ClientAuth: got %v want %v", cfg.ClientAuth, want)
	}
}
