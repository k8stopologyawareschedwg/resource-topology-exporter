package prometheus

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const prometheusDefaultPort = "2112"

var nodeName string

var (
	PodResourceApiCallsFailure = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "podresource_api_call_failures_total",
		Help: "The total number of podresource api calls that failed by the updater",
	}, []string{"node", "function_name"})

	OperationDelay = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "operation_delay",
			Help: "Represent the latency of the update operation",
		}, []string{"node", "operation_name"})
)

func getNodeName() (string, error) {
	var err error

	val, ok := os.LookupEnv("NODE_NAME")
	if !ok {
		val, err = os.Hostname()
		if err != nil {
			return "", err
		}
	}
	return val, nil
}

func UpdatePodResourceApiCallsFailureMetric(funcName string) {
	PodResourceApiCallsFailure.With(prometheus.Labels{
		"node":      nodeName,
		"function_name": funcName,
	}).Inc()
}

func UpdateOperationDelayMetric(opName string, operationDelay float64) {
	OperationDelay.With(prometheus.Labels{
		"node": nodeName,
		"operation_name": opName,
	}).Set(operationDelay)
}

func InitPrometheus() error {
	var err error
	var port = prometheusDefaultPort

	if envValue, ok := os.LookupEnv("METRICS_PORT"); ok {
		if _, err = strconv.Atoi(envValue); err != nil {
			return fmt.Errorf("the env variable PROMETHEUS_PORT has inccorrect value %q; err %v", envValue, err)
		}
		port = envValue
	}

	nodeName, err = getNodeName()
	if err != nil {
		return err
	}

	http.Handle("/metrics", promhttp.Handler())
	addr := fmt.Sprintf("0.0.0.0:%s", port)

	go func() {
		if err = http.ListenAndServe(addr, nil); err != nil {
			log.Fatalf("failed to run prometheus server; %v", err)
		}
	}()

	return nil
}
