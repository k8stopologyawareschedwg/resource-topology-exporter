package wait

import (
	"context"
	"fmt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	PodPollInterval = time.Second * 10
	PodPollTimeout  = time.Minute * 2
)

func ForPodToBeRunning(ctx context.Context, c client.Client, p *corev1.Pod) error {
	ready, phase, err := ForPodPhase(ctx, c, p, corev1.PodRunning)
	if ready {
		return nil
	}
	return fmt.Errorf("pod=%q is not Running; phase=%q; %w", client.ObjectKeyFromObject(p), phase, err)
}

func ForPodToBeDeleted(ctx context.Context, c client.Client, key client.ObjectKey) error {
	err := wait.PollImmediate(PodPollInterval, PodPollTimeout, func() (done bool, err error) {
		p := &corev1.Pod{}
		err = c.Get(ctx, key, p)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("pod=%q was no deleted; %w", key, err)
	}
	return nil
}

func ForPodPhase(ctx context.Context, c client.Client, p *corev1.Pod, desiredPhase corev1.PodPhase) (bool, corev1.PodPhase, error) {
	var podPhase corev1.PodPhase
	err := wait.PollImmediate(PodPollInterval, PodPollTimeout, func() (done bool, err error) {
		err = c.Get(ctx, client.ObjectKeyFromObject(p), p)
		if err != nil {
			return false, err
		}

		podPhase = p.Status.Phase
		if podPhase == desiredPhase {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return false, podPhase, err
	}
	return true, podPhase, err
}
