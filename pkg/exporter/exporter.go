package exporter

import (
	"fmt"
	"github.com/swatisehgal/resource-topology-exporter/pkg/apis/topocontroller/v1alpha1"
	"log"
)

type crdExporter struct {
	NodeResources []v1alpha1.NUMANodeResource
}

type CRDExporter interface {
	Run() error
}

func NewExporter(instance []v1alpha1.NUMANodeResource) *crdExporter {
	return &crdExporter{
		NodeResources: instance,
	}
}

func (e *crdExporter) Run() error {
	err := e.Update()
	if err != nil {
		return fmt.Errorf("Unable to update Exporter: %v", err)
	}
	return nil
}

func (e *crdExporter) Update() error {
	log.Printf("Exporter Update called NodeResources is: %+v", e.NodeResources)

	err := UpdateCustomResourceDefinition(e.NodeResources)
	if err != nil {
		return fmt.Errorf("Unable to update CRD: %v", err)
	}
	return nil
}
