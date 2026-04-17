package podreadiness

import (
	"context"
	"fmt"
	"os"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

type ConditionInjector struct {
	cs      kubernetes.Interface
	ns      string
	podName string
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

	return &ConditionInjector{
		cs:      cs,
		ns:      nsVal,
		podName: podVal,
	}, nil
}

func (ci *ConditionInjector) Inject(ctx context.Context, cond v1.PodCondition) error {
	conditionExist := false
	pod, err := ci.cs.CoreV1().Pods(ci.ns).Get(ctx, ci.podName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	for pos, podCond := range pod.Status.Conditions {
		if podCond.Type == cond.Type {
			conditionExist = true
			if podCond.Status == cond.Status {
				// do nothing condition already updated
				return nil
			}
			// update the condition
			pod.Status.Conditions[pos] = cond
		}
	}

	if !conditionExist {
		conds := pod.Status.Conditions
		conds = append(conds, cond)
		pod.Status.Conditions = conds
	}

	klog.V(4).Infof("pod conditions: %v", pod.Status.Conditions)

	_, err = ci.cs.CoreV1().Pods(ci.ns).UpdateStatus(ctx, pod, metav1.UpdateOptions{})
	return err
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
