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

	"k8s.io/klog/v2"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	tlsRootDir = "/etc/secrets/rte"

	tlsCert = tlsRootDir + "/tls.crt"
	tlsKey  = tlsRootDir + "/tls.key"
	caCert  = tlsRootDir + "/ca.crt"
)

func NewDefaultTLSConfig() TLSConfig {
	return TLSConfig{
		CertFile:    tlsCert,
		KeyFile:     tlsKey,
		CACertFile:  caCert,
		WantCliAuth: true,
	}
}

type TLSServer struct {
	srv     *http.Server
	tlsConf TLSConfig
}

func SetupHTTPTLS(conf Config, ctx context.Context) error {
	srv, err := makeTLSServer(conf)
	if err != nil {
		return fmt.Errorf("failed to build server with port %d: %w", conf.Port, err)
	}

	go srv.Start()

	return nil
}

func makeTLSServer(conf Config) (*TLSServer, error) {
	handler := promhttp.HandlerFor(
		conf.Gatherer,
		promhttp.HandlerOpts{
			ErrorHandling: promhttp.HTTPErrorOnError,
		},
	)

	tlsConfig := &tls.Config{}
	caCert, err := os.ReadFile(conf.TLS.CACertFile)
	if err != nil {
		klog.Errorf("failed to read %q: %v", conf.TLS.CACertFile, err)
		return nil, err
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		klog.Error("failed to parse %q", conf.TLS.CACertFile)
		return nil, err
	} else {
		tlsConfig.ClientCAs = caCertPool
		tlsConfig.MinVersion = tls.VersionTLS12
		tlsConfig.CipherSuites = []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		}
		tlsConfig.NextProtos = []string{"http/1.1"}
	}
	if conf.TLS.WantCliAuth {
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	if tlsConfig.ClientCAs == nil {
		// TODO
		klog.Infof("continuing without client authentication")
	}

	router := http.NewServeMux()
	router.Handle("/metrics", handler)

	return &TLSServer{
		srv: &http.Server{
			Addr:      conf.Address(),
			Handler:   router,
			TLSConfig: tlsConfig,
		},
		tlsConf: conf.TLS,
	}, nil
}

func (tlssrv *TLSServer) Start() {
	klog.Infof("starting secure metrics server")
	err := tlssrv.srv.ListenAndServeTLS(tlssrv.tlsConf.CertFile, tlssrv.tlsConf.KeyFile)
	if err != nil && err != http.ErrServerClosed {
		klog.Errorf("error from secure metrics server: %v", err)
	}
}
