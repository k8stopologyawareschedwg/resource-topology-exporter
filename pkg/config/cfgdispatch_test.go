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

package config

import (
	"reflect"
	"testing"
)

// TestDispatchCoversAllFields uses reflection to walk every field in
// the ProgArgs struct and verify it has a corresponding entry in the
// config-file dispatch table as returned by MakeBindings().
// Fields that are intentionally handled elsewhere must appear in the
// internal skip list with a reason.
//
// This catches the class of bug where a new struct field is added and
// wired as a CLI flag but not in the config-file dispatch table.
func TestDispatchCoversAllFields(t *testing.T) {
	var pArgs ProgArgs
	SetDefaults(&pArgs)

	// Fields intentionally absent from the dispatch table.
	// Every entry documents WHY the field is not dispatched.
	skipFields := map[string]string{
		"Version":                         "CLI-only, not a config file option",
		"DumpConfig":                      "CLI-only, not a config file option",
		"NRTupdater.KubeConfig":           "unused; Global.KubeConfig is used instead",
		"Resourcemonitor.ResourceExclude": "loaded from extra config file, not daemon config",
		"Resourcemonitor.PodExclude":      "loaded from extra config file, not daemon config",
		"RTE.ReferenceContainer":          "special-case dispatch with string parsing",
		"RTE.MaxEventsPerTimeUnit":        "special-case dispatch with int64 conversion",
		"RTE.TopologyManagerPolicy":       "loaded from extra config file",
		"RTE.TopologyManagerScope":        "loaded from extra config file",
	}

	bindings := MakeBindings(&pArgs)
	dispatchAddrs := make(map[uintptr]string, len(bindings))
	for _, b := range bindings {
		addr := reflect.ValueOf(b.out).Pointer()
		dispatchAddrs[addr] = b.key
	}

	walkStruct(reflect.ValueOf(&pArgs).Elem(), "", func(path string, addr uintptr) {
		if reason, ok := skipFields[path]; ok {
			t.Logf("dispatch skip %s: %s", path, reason)
			return
		}
		if _, ok := dispatchAddrs[addr]; !ok {
			t.Errorf("field %s has no entry in the config dispatch table and is not in the skip list.\n"+
				"Add an entry in MakeBindings() (cfgdispatch.go) or add %s to skipFields with a reason.", path, path)
		}
	})
}

// walkStruct walks all exported leaf fields of a struct value,
// recursing into nested structs. For each leaf it calls fn with the
// dotted path (e.g. "RTE.MetricsTLSCfg.CertsDir") and the field's address.
func walkStruct(v reflect.Value, prefix string, fn func(path string, addr uintptr)) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		fv := v.Field(i)
		path := field.Name
		if prefix != "" {
			path = prefix + "." + field.Name
		}

		if fv.Kind() == reflect.Struct {
			walkStruct(fv, path, fn)
			continue
		}

		if !fv.CanAddr() {
			continue
		}

		fn(path, fv.Addr().Pointer())
	}
}
