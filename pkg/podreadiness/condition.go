package podreadiness

import (
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RTEConditionType is a valid value for PodCondition.Type
type RTEConditionType string

// These are valid conditions of RTE pod.
const (
	// PodresourcesFetched means that resources scanned successfully.
	PodresourcesFetched RTEConditionType = "PodresourcesFetched"
	// NodeTopologyUpdated means that noderesourcetopology objects updated successfully.
	NodeTopologyUpdated RTEConditionType = "NodeTopologyUpdated"
)

func SetCondition(condChan chan<- v1.PodCondition, condType RTEConditionType, condStatus v1.ConditionStatus) {
	if condChan == nil {
		return
	}
	cond := newConditionTemplate(condType, condStatus)
	if condStatus == v1.ConditionFalse {
		switch condType {
		case PodresourcesFetched:
			cond.Reason = "ScanFailed"
			cond.Message = "failed to scan pod resources"
		case NodeTopologyUpdated:
			cond.Reason = "UpdateFailed"
			cond.Message = "failed to update noderesourcetopology object"
		}
	}
	condChan <- cond
}

func newConditionTemplate(condType RTEConditionType, status v1.ConditionStatus) (condition v1.PodCondition) {
	return v1.PodCondition{
		Type:               v1.PodConditionType(condType),
		Status:             status,
		LastTransitionTime: metav1.Time{Time: time.Now()},
	}
}
