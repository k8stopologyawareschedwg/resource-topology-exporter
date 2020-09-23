package podres

import (
	"fmt"
	"log"
	"time"

	podresources "k8s.io/kubernetes/pkg/kubelet/apis/podresources"
	podresourcesapi "k8s.io/kubernetes/pkg/kubelet/apis/podresources/v1alpha1"
)

const (
	defaultPodResourcesTimeout = 10 * time.Second
	defaultPodResourcesMaxSize = 1024 * 1024 * 16 // 16 Mb
	// obtained these values from node e2e tests : https://github.com/kubernetes/kubernetes/blob/82baa26905c94398a0d19e1b1ecf54eb8acb6029/test/e2e_node/util.go#L70
)

func GetPodResClient(socketPath string) (podresourcesapi.PodResourcesListerClient, error) {

	var err error
	podResourceClient, _, err := podresources.GetClient(socketPath, defaultPodResourcesTimeout, defaultPodResourcesMaxSize)
	if err != nil {
		return nil, fmt.Errorf("Can't create client: %v", err)
	}
	log.Printf("connected to '%v'!", socketPath)
	return podResourceClient, nil
}
