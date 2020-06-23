package exporter

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	v1alpha1 "github.com/swatisehgal/resource-topology-exporter/pkg/apis/topocontroller/v1alpha1"
	clientset "github.com/swatisehgal/resource-topology-exporter/pkg/generated/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"log"
	"os"
)
var (
	created = false
)
func UpdateCustomResourceDefinition(resources []v1alpha1.NUMANodeResource) error{
	clientConfig, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("Please run from inside the cluster")
	}

	exampleClient, err := clientset.NewForConfig(clientConfig)
	if err != nil {
			return fmt.Errorf("Error building example clientset: %s", err.Error())
	}
	topocontroller := exampleClient.TopocontrollerV1alpha1()

	if topocontroller == nil {
		return fmt.Errorf("Can't get TopocontrollerV1alpha1")
	}
	namespace := "default"

	resourceTopology := topocontroller.NodeResourceTopologies(namespace)

	if resourceTopology == nil {
			return fmt.Errorf("Can't get resource topology interface")
	}

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("Failed to get Hostname:%v", err)
	}
	if !created{
		resTopo, err := resourceTopology.Create(&v1alpha1.NodeResourceTopology{
			ObjectMeta: metav1.ObjectMeta{
				Name: hostname,
			},
			Nodes: resources,
		}, metav1.CreateOptions{})

		if err != nil {
			return fmt.Errorf("Failed to create v1alpha1.NodeResourceTopology!:%v", err)
		}
		created=true
			log.Printf("CRD instance created resTopo: %v", spew.Sdump(resTopo))
	}

	resTopo, err := resourceTopology.Get(hostname, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("Failed to Get v1alpha1.NodeResourceTopology!:%v", err)
	}
	resVersion := resTopo.ObjectMeta.ResourceVersion

	resTopo, err = resourceTopology.Update(&v1alpha1.NodeResourceTopology{
		ObjectMeta: metav1.ObjectMeta{
			Name: hostname,
			ResourceVersion: resVersion,
		},
		Nodes: resources,
	}, metav1.UpdateOptions{})

	if err != nil {
	return fmt.Errorf("Failed to update v1alpha1.NodeResourceTopology!:%v", err)
	}

	log.Printf("CRD instance updated resTopo: %v", resTopo)
	return nil
}
