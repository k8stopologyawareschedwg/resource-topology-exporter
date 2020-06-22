package exporter

import (
	v1alpha1 "github.com/swatisehgal/resource-topology-exporter/pkg/apis/topocontroller/v1alpha1"
	clientset "github.com/swatisehgal/resource-topology-exporter/pkg/generated/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"log"
)

func CreateCustomResourceDefinition(resources []v1alpha1.NUMANodeResource) {
	clientConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Print("not in cluster - trying file-based configuration")
	}

	exampleClient, err := clientset.NewForConfig(clientConfig)
	if err != nil {
		log.Fatalf("Error building example clientset: %s", err.Error())
	}
	topocontroller := exampleClient.TopocontrollerV1alpha1()

	if topocontroller == nil {
		log.Fatalf("Can't get TopocontrollerV1alpha1")
	}
	namespace := "default"

	resourceTopology := topocontroller.NodeResourceTopologies(namespace)

	if resourceTopology == nil {
		log.Fatalf("Can't get resource topology interface!")
	}

	resTopo, err := resourceTopology.Create(&v1alpha1.NodeResourceTopology{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-test1",
		},
		Nodes: resources,
	}, metav1.CreateOptions{})

	if err != nil {
		log.Fatalf("Failed to create v1alpha1.NodeResourceTopology!:%v", err)
	}

	log.Printf("resTopo: %v", resTopo)

}
