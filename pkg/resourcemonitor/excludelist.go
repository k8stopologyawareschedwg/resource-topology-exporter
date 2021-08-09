package resourcemonitor

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
)

// ToMapSet keeps the original keys, but replaces values with set.String types
func (r *ResourceExcludeList) ToMapSet() map[string]sets.String {
	asSet := make(map[string]sets.String)
	for k, v := range r.ExcludeList {
		asSet[k] = sets.NewString(v...)
	}
	return asSet
}

func (r *ResourceExcludeList) String() string {
	var b strings.Builder
	for name, items := range r.ExcludeList {
		fmt.Fprintf(&b, "- %s: [%s]\n", name, strings.Join(items, ", "))
	}
	return b.String()
}
