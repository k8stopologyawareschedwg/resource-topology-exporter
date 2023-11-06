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
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog/v2"
)

const prometheusDefaultPort = 2112

type Config struct {
	Port       int
	Registerer prometheus.Registerer
	Gatherer   prometheus.Gatherer
}

func NewDefaultConfig() Config {
	return Config{
		Port:       prometheusDefaultPort,
		Registerer: prometheus.DefaultRegisterer,
		Gatherer:   prometheus.DefaultGatherer,
	}
}

func (conf Config) Address() string {
	return fmt.Sprintf("0.0.0.0:%d", conf.Port)
}

func (conf Config) Validate() error {
	if conf.Port <= 0 {
		return fmt.Errorf("invalid port: %d", conf.Port)
	}
	return nil
}

const (
	ServingDisabled = "disabled"
	ServingHTTP     = "http" // plaintext
)

func ServingModeIsSupported(value string) (string, error) {
	val := strings.ToLower(value)
	switch val {
	case ServingDisabled:
		return val, nil
	case ServingHTTP:
		return val, nil
	default:
		return val, fmt.Errorf("unsupported method  %q", value)
	}
}

func ServingModeSupported() string {
	modes := []string{
		ServingDisabled,
		ServingHTTP,
	}
	return strings.Join(modes, ",")
}

func Setup(mode string, conf Config) error {
	if mode == ServingDisabled {
		klog.Infof("metrics endpoint disabled")
		return nil
	}

	if envValue, ok := os.LookupEnv("METRICS_PORT"); ok {
		if _, err := strconv.Atoi(envValue); err != nil {
			return fmt.Errorf("the env variable PROMETHEUS_PORT has inccorrect value %q: %w", envValue, err)
		}
		port, err := strconv.Atoi(envValue)
		if err != nil {
			return err
		}

		klog.V(2).InfoS("overriding metrics port", "from", conf.Port, "to", port)
		conf.Port = port
	}

	if err := conf.Validate(); err != nil {
		return err
	}

	if mode == ServingHTTP {
		return SetupHTTP(conf)
	}

	return fmt.Errorf("unknown mode: %v", mode)
}

func SetupHTTP(conf Config) error {
	http.Handle("/metrics", promhttp.InstrumentMetricHandler(
		conf.Registerer, promhttp.HandlerFor(conf.Gatherer, promhttp.HandlerOpts{}),
	))

	go func() {
		err := http.ListenAndServe(conf.Address(), nil)
		if err != nil {
			klog.Fatalf("failed to run prometheus server; %v", err)
		}
	}()

	return nil
}
