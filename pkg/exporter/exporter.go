package exporter

import (
	"fmt"
	"log"
	"os"

	"github.com/davecgh/go-spew/spew"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	v1alpha1 "github.com/swatisehgal/resource-topology-exporter/pkg/apis/topocontroller/v1alpha1"
	clientset "github.com/swatisehgal/resource-topology-exporter/pkg/generated/clientset/versioned"
)

type CRDExporter struct {
	cli      *clientset.Clientset
	hostname string
}

func NewExporter() (*CRDExporter, error) {
	clientConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("Please run from inside the cluster")
	}

	cli, err := clientset.NewForConfig(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("Error building example clientset: %s", err.Error())
	}

	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("Failed to get Hostname:%v", err)
	}

	return &CRDExporter{
		cli:      cli,
		hostname: hostname,
	}, nil
}

func (e *CRDExporter) CreateOrUpdate(namespace string, resources []v1alpha1.NUMANodeResource) error {
	log.Printf("Exporter Update called NodeResources is: %+v", resources)

	nrt, err := e.cli.TopocontrollerV1alpha1().NodeResourceTopologies(namespace).Get(e.hostname, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		nrtNew := v1alpha1.NodeResourceTopology{
			ObjectMeta: metav1.ObjectMeta{
				Name: e.hostname,
			},
			Nodes: resources,
		}

		nrtCreated, err := e.cli.TopocontrollerV1alpha1().NodeResourceTopologies(namespace).Create(&nrtNew, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("Failed to create v1alpha1.NodeResourceTopology!:%v", err)
		}
		log.Printf("CRD instance created resTopo: %v", spew.Sdump(nrtCreated))
		return nil
	}

	if err != nil {
		return err
	}

	nrtMutated := nrt.DeepCopy()
	nrtMutated.Nodes = resources

	nrtUpdated, err := e.cli.TopocontrollerV1alpha1().NodeResourceTopologies(namespace).Update(nrtMutated, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("Failed to update v1alpha1.NodeResourceTopology!:%v", err)
	}
	log.Printf("CRD instance updated resTopo: %v", nrtUpdated)
	return nil
}
