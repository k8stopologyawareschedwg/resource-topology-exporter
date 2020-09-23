package exporter

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/davecgh/go-spew/spew"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	v1alpha1 "github.com/swatisehgal/topologyapi/pkg/apis/topology/v1alpha1"
	clientset "github.com/swatisehgal/topologyapi/pkg/generated/clientset/versioned"
)

type CRDExporter struct {
	cli                   *clientset.Clientset
	hostname              string
	topologyManagerPolicy string
}

func NewExporter(tmPolicy string) (*CRDExporter, error) {
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
		cli:                   cli,
		hostname:              hostname,
		topologyManagerPolicy: tmPolicy,
	}, nil
}

func (e *CRDExporter) CreateOrUpdate(namespace string, zones v1alpha1.ZoneMap) error {
	log.Printf("Exporter Update called NodeResources is: %+v", zones)

	nrt, err := e.cli.TopologyV1alpha1().NodeResourceTopologies(namespace).Get(context.TODO(), e.hostname, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		nrtNew := v1alpha1.NodeResourceTopology{
			ObjectMeta: metav1.ObjectMeta{
				Name: e.hostname,
			},
			Zones: zones,
			TopologyPolicy: []string{
				e.topologyManagerPolicy,
			},
		}

		nrtCreated, err := e.cli.TopologyV1alpha1().NodeResourceTopologies(namespace).Create(context.TODO(), &nrtNew, metav1.CreateOptions{})
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
	nrtMutated.Zones = zones

	nrtUpdated, err := e.cli.TopologyV1alpha1().NodeResourceTopologies(namespace).Update(context.TODO(), nrtMutated, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("Failed to update v1alpha1.NodeResourceTopology!:%v", err)
	}
	log.Printf("CRD instance updated resTopo: %v", nrtUpdated)
	return nil
}
