package podreadiness

import (
	"context"
	"fmt"
	"os"
	"slices"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

type ConditionInjector struct {
	cs         kubernetes.Interface
	ns         string
	podName    string
	lastUpdate map[v1.PodConditionType]v1.PodCondition
}

func NewConditionInjectorWithIdentity(cs kubernetes.Interface, ns, podName string) *ConditionInjector {
	return &ConditionInjector{
		cs:         cs,
		ns:         ns,
		podName:    podName,
		lastUpdate: make(map[v1.PodConditionType]v1.PodCondition),
	}
}

func NewConditionInjector(cs kubernetes.Interface) (*ConditionInjector, error) {
	nsVal, ok := os.LookupEnv("REFERENCE_NAMESPACE")
	if !ok {
		return nil, fmt.Errorf("the env REFERENCE_NAMESPACE doesn't exist")
	}

	podVal, ok := os.LookupEnv("REFERENCE_POD_NAME")
	if !ok {
		return nil, fmt.Errorf("the env REFERENCE_POD_NAME doesn't exist")
	}
	return NewConditionInjectorWithIdentity(cs, nsVal, podVal), nil
}

func (ci *ConditionInjector) Inject(ctx context.Context, cond v1.PodCondition) error {
	if oldCond, ok := ci.lastUpdate[cond.Type]; ok && equalPodCondition(oldCond, cond) {
		// avoid APIServer round trips if nothing changed
		return nil
	}

	pod, err := ci.cs.CoreV1().Pods(ci.ns).Get(ctx, ci.podName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	pos := slices.IndexFunc(pod.Status.Conditions, func(c v1.PodCondition) bool {
		return c.Type == cond.Type
	})
	if pos >= 0 {
		// note this cause an unnecessary update on RTE restart at steady state.
		// we can re-update conditions with the existing value because of that.
		// we expect RTE restarts to be rare, so we tolerate this extra write
		// for now.
		pod.Status.Conditions[pos] = cond
		klog.V(4).Infof("updated conditions inplace: %v", pod.Status.Conditions)
	} else {
		pod.Status.Conditions = append(pod.Status.Conditions, cond)
		klog.V(4).Infof("updated conditions adding: %v", pod.Status.Conditions)
	}

	_, err = ci.cs.CoreV1().Pods(ci.ns).UpdateStatus(ctx, pod, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	ci.lastUpdate[cond.Type] = cond
	return nil
}

func (ci *ConditionInjector) Run(ctx context.Context, condChan <-chan v1.PodCondition) {
	for {
		select {
		case cond := <-condChan:
			err := ci.Inject(ctx, cond)
			if err != nil {
				klog.Errorf("failed to update pod status with condition: %v", cond)
			}
		case <-ctx.Done():
			return
		}
	}
}

func equalPodCondition(a, b v1.PodCondition) bool {
	return a.Type == b.Type && a.Status == b.Status && a.Reason == b.Reason && a.Message == b.Message
}
