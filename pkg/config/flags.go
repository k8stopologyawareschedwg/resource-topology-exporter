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
	"strings"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/version"
)

func setupFlags() (*flag.FlagSet, *string, func(key string, value string) (string, interface{})) {
	flags := flag.NewFlagSet(version.ProgramName, flag.ExitOnError)

	// rmap = Reverse MAP: name -> group
	rmap := map[string]string{}

	// updater
	addFlagBool(flags, rmap, "updater", "no-publish", false, "do not publish discovered features to the cluster-local Kubernetes API server.")
	addFlagBool(flags, rmap, "updater", "oneshot", false, "update once and exit.")
	addFlagString(flags, rmap, "updater", "hostname", DefaultHostName(), "override the node hostname.")

	// monitor
	addFlagString(flags, rmap, "monitor", "watch-namespace", "", "namespace to watch pods for. Use \"\" for all namespaces.")
	addFlagString(flags, rmap, "monitor", "sysfs", "/sys", "top-level component path of sysfs.")
	addFlagBool(flags, rmap, "monitor", "pods-fingerprint", false, "if enable, compute and report the pod set fingerprint.")
	addFlagBool(flags, rmap, "monitor", "expose-timing", false, "if enable, expose expected and actual sleep interval as annotations.")
	addFlagBool(flags, rmap, "monitor", "refresh-node-resources", false, "if enable, track changes in node's resources")

	// exporter
	addFlagString(flags, rmap, "exporter", "topology-manager-policy", DefaultTopologyManagerPolicy(), "explicitly set the topology manager policy instead of reading from the kubelet.")
	addFlagString(flags, rmap, "exporter", "topology-manager-scope", DefaultTopologyManagerScope(), "explicitly set the topology manager scope instead of reading from the kubelet.")
	addFlagDuration(flags, rmap, "exporter", "sleep-interval", 60*time.Second, "time to sleep between podresources API polls.")
	addFlagString(flags, rmap, "exporter", "kubelet-config-file", "/podresources/config.yaml", "kubelet config file path.")
	addFlagString(flags, rmap, "exporter", "podresources-socket", "unix:///podresources/kubelet.sock", "pod resources Socket path to use.")
	addFlagBool(flags, rmap, "exporter", "podreadiness", true, "custom condition injection using podreadiness.")

	addFlagString(flags, rmap, "exporter", "kubelet-state-dir", "", "kubelet state directory (RO access needed), for smart polling.")
	addFlagString(flags, rmap, "exporter", "reference-container", "", "reference container, used to learn about the shared cpu pool\n See: https://github.com/kubernetes/kubernetes/issues/102190\n format of spec is namespace/podname/containername.\n Alternatively, you can use the env vars REFERENCE_NAMESPACE, REFERENCE_POD_NAME, REFERENCE_CONTAINER_NAME.")

	addFlagString(flags, rmap, "exporter", "notify-file", "", "notification file path.")
	// Lets keep it simple by now and expose only "events-per-second"
	// but logic is prepared to be able to also define the time base
	// that is why TimeUnitToLimitEvents is hard initialized to Second
	addFlagInt64(flags, rmap, "exporter", "max-events-per-second", 1, "max times per second resources will be scanned and updated")

	// generic
	conf := flags.String("config", "", "comma-separated path(s) to one or more yaml config files")
	flags.Bool("debug", false, "enable debug output.")
	flags.Bool("version", false, "output version and exit")
	flags.String("dump-config", "",
		`dump the current configuration to the given file path.
Special targets:
. "-" for stdout.
. ".andexit" stdout and exit right after.
. ".log" to dump in the log".`,
	)

	keyMapper := func(key string, value string) (string, interface{}) {
		group, ok := rmap[key]
		if !ok {
			group = "core"
		}
		return group + keyDelim + key, value
	}

	return flags, conf, keyMapper
}

func splitConfigFiles(conf *string) []string {
	var ret []string
	if conf == nil || *conf == "" {
		return ret
	}
	for _, item := range strings.Split(*conf, ",") {
		ret = append(ret, strings.TrimSpace(item))
	}
	return ret
}

func addFlagBool(f *flag.FlagSet, rmap map[string]string, group, name string, value bool, usage string) *bool {
	rmap[name] = group
	return f.Bool(name, value, "["+group+"]: "+usage)
}

func addFlagString(f *flag.FlagSet, rmap map[string]string, group, name string, value string, usage string) *string {
	rmap[name] = group
	return f.String(name, value, "["+group+"]: "+usage)
}

func addFlagDuration(f *flag.FlagSet, rmap map[string]string, group, name string, value time.Duration, usage string) *time.Duration {
	rmap[name] = group
	return f.Duration(name, value, "["+group+"]: "+usage)
}

func addFlagInt64(f *flag.FlagSet, rmap map[string]string, group, name string, value int64, usage string) *int64 {
	rmap[name] = group
	return f.Int64(name, value, "["+group+"]: "+usage)
}
