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

package metrics

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"

	"k8s.io/klog/v2"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"gopkg.in/fsnotify.v1"
)

const (
	tlsSecretDir = "/etc/secrets"
	tlsCert      = tlsSecretDir + "/tls.crt"
	tlsKey       = tlsSecretDir + "/tls.key"

	authCADir  = "/tmp/metrics-client-ca"
	authCAFile = authCADir + "/ca.crt"
)

type TLSServer struct{}

func SetupHTTPTLS(conf Config, ctx context.Context) error {
	// Set up and start the file watcher.
	watcher, err := fsnotify.NewWatcher()
	if watcher == nil || err != nil {
		klog.Errorf("failed to create file watcher, cert/key rotation will be disabled %v", err)
	} else {
		defer watcher.Close()

		if err = watcher.Add(authCADir); err != nil {
			klog.Errorf("failed to add %v to watcher, CA client authentication and rotation will be disabled: %v", authCADir, err)
		} else {
			waitForCAFile(ctx, watcher)
		}

		if err = watcher.Add(tlsSecretDir); err != nil {
			klog.Errorf("failed to add %v to watcher, cert/key rotation will be disabled: %v", tlsSecretDir, err)
		}
	}

	srv, err := makeTLSServer(conf)
	if err != nil {
		return fmt.Errorf("failed to build server with port %d: %w", conf.Port, err)
	}

	go startTLSServer(srv)

	orig := newChecksums()

	for {
		select {
		case <-ctx.Done():
			stopTLSServer(srv)
			return nil
		case event := <-watcher.Events:
			klog.V(2).Infof("event from filewatcher on file: %v, event: %v", event.Name, event.Op)

			if event.Op == fsnotify.Chmod || event.Op == fsnotify.Remove {
				continue
			}

			if !allFilesReady() {
				continue
			}

			current := newChecksums()
			if !current.IsValid() {
				continue // TODO
			}
			if orig.Equal(current) {
				continue // TODO
			}

			orig = current

			klog.Infof("restarting metrics server to rotate certificates")
			stopTLSServer(srv)
			srv, err = makeTLSServer(conf)
			if err != nil {
				return err
			}
			go startTLSServer(srv)
		case err = <-watcher.Errors:
			klog.Warningf("error from metrics server certificate file watcher: %v", err)
		}
	}
	return nil
}

func makeTLSServer(conf Config) (*http.Server, error) {
	handler := promhttp.HandlerFor(
		conf.Gatherer,
		promhttp.HandlerOpts{
			ErrorHandling: promhttp.HTTPErrorOnError,
		},
	)

	tlsConfig := &tls.Config{}
	caCert, err := os.ReadFile(authCAFile)
	if err != nil {
		klog.Errorf("failed to read %q: %v", authCAFile, err)
		return nil, err
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		klog.Error("failed to parse %q", authCAFile)
		return nil, err
	} else {
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		tlsConfig.ClientCAs = caCertPool
		tlsConfig.MinVersion = tls.VersionTLS12
		tlsConfig.CipherSuites = []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		}
		tlsConfig.NextProtos = []string{"http/1.1"}
	}

	if tlsConfig.ClientCAs == nil {
		// TODO
		klog.Infof("continuing without client authentication")
	}

	router := http.NewServeMux()
	router.Handle("/metrics", handler)
	srv := &http.Server{
		Addr:      conf.Address(),
		Handler:   router,
		TLSConfig: tlsConfig,
	}

	return srv, nil
}

func startTLSServer(srv *http.Server) {
	klog.Infof("starting metrics server")
	err := srv.ListenAndServeTLS(tlsCert, tlsKey)
	if err != nil && err != http.ErrServerClosed {
		klog.Errorf("error from metrics server: %v", err)
	}
}

func stopTLSServer(srv *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	klog.Infof("stopping metrics server")
	err := srv.Shutdown(ctx)
	if err != nil {
		klog.Errorf("error or timeout stopping metrics listener: %v", err)
	}
}

func waitForCAFile(ctx context.Context, watcher *fsnotify.Watcher) error {
	ok, _ := fileExistsAndNotEmpty(authCAFile)
	if ok {
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case event := <-watcher.Events:
			klog.V(2).Infof("event from filewatcher on file: %q, event: %q", event.Name, event.Op)

			if event.Name != authCAFile {
				continue
			}
			ok, _ := fileExistsAndNotEmpty(authCAFile)
			if !ok {
				klog.V(2).Infof("event from filewatcher on file: %q but still empty", event.Name)
				continue
			}
			klog.V(3).Infof("wait completed for %q", event.Name)
			return nil
		case err := <-watcher.Errors:
			klog.Warningf("error from metrics server CA client authentication file watcher: %v", err)
			return err
		}
	}
	return nil
}
