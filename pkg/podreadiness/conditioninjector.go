package podreadiness

import (
	"context"
	"fmt"
	"os"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/k8shelpers"
)

type ConditionInjector struct {
	cs      *kubernetes.Clientset
	ns      string
	podName string
}

func NewConditionInjector() (*ConditionInjector, error) {
	nsVal, ok := os.LookupEnv("REFERENCE_NAMESPACE")
	if !ok {
		return nil, fmt.Errorf("the env REFERENCE_NAMESPACE doesn't exist")
	}

	podVal, ok := os.LookupEnv("REFERENCE_POD_NAME")
	if !ok {
		return nil, fmt.Errorf("the env REFERENCE_POD_NAME doesn't exist")
	}

	cs, err := k8shelpers.GetK8sClient("")
	if err != nil {
		return nil, err
	}

	return &ConditionInjector{
		cs:      cs,
		ns:      nsVal,
		podName: podVal,
	}, nil
}

func (ci *ConditionInjector) Inject(cond v1.PodCondition) error {
	conditionExist := false
	pod, err := ci.cs.CoreV1().Pods(ci.ns).Get(context.TODO(), ci.podName, metav1.GetOptions{})
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

	klog.Infof("pod conditions: %v", pod.Status.Conditions)

	_, err = ci.cs.CoreV1().Pods(ci.ns).UpdateStatus(context.TODO(), pod, metav1.UpdateOptions{})
	return err
}

func (ci *ConditionInjector) Run(condChan <-chan v1.PodCondition) {
	go func() {
		for {
			cond := <-condChan
			err := ci.Inject(cond)
			if err != nil {
				klog.Errorf("failed to update pod status with condition: %v", cond)
			}
		}
	}()
}
