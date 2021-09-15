package podres

import (
	"fmt"
	"log"
	"time"

	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"
	"k8s.io/kubernetes/pkg/kubelet/apis/podresources"
)

const (
	// obtained the following values from node e2e tests : https://github.com/kubernetes/kubernetes/blob/82baa26905c94398a0d19e1b1ecf54eb8acb6029/test/e2e_node/util.go#L70
	defaultPodResourcesTimeout = 10 * time.Second
	defaultPodResourcesMaxSize = 1024 * 1024 * 16 // 16 Mb
)

func GetPodResClient(socketPath string) (podresourcesapi.PodResourcesListerClient, error) {
	podResourceClient, _, err := podresources.GetV1Client(socketPath, defaultPodResourcesTimeout, defaultPodResourcesMaxSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create podresource client: %w", err)
	}
	log.Printf("Connected to '%q'!", socketPath)
	return podResourceClient, nil
}
