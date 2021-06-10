package nrtupdater

import (
	"context"
	"fmt"
	"log"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/dumpobject"
)

var (
	stdoutLogger = log.New(os.Stdout, "", log.LstdFlags)
	stderrLogger = log.New(os.Stderr, "", log.LstdFlags)
)

// Command line arguments
type Args struct {
	NoPublish bool
	Oneshot   bool
	Hostname  string
	Namespace string
}

type NRTUpdater struct {
	args     Args
	tmPolicy string
}

func NewNRTUpdater(args Args, policy string) (*NRTUpdater, error) {
	te := &NRTUpdater{
		args:     args,
		tmPolicy: policy,
	}
	return te, nil
}

func (te *NRTUpdater) Update(zones v1alpha1.ZoneList) error {
	stdoutLogger.Printf("update: sending zone: '%s'", dumpobject.DumpObject(zones))

	if te.args.NoPublish {
		return nil
	}

	cli, err := GetTopologyClient("")
	if err != nil {
		return err
	}

	hostname := te.args.Hostname   // shortcut
	namespace := te.args.Namespace // shortcut

	nrt, err := cli.TopologyV1alpha1().NodeResourceTopologies(namespace).Get(context.TODO(), hostname, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		nrtNew := v1alpha1.NodeResourceTopology{
			ObjectMeta: metav1.ObjectMeta{
				Name: hostname,
			},
			Zones:            zones,
			TopologyPolicies: []string{te.tmPolicy},
		}

		nrtCreated, err := cli.TopologyV1alpha1().NodeResourceTopologies(namespace).Create(context.TODO(), &nrtNew, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("Failed to create v1alpha1.NodeResourceTopology!:%v", err)
		}
		log.Printf("CRD instance created resTopo: %v", dumpobject.DumpObject(nrtCreated))
		return nil
	}

	if err != nil {
		return err
	}

	nrtMutated := nrt.DeepCopy()
	nrtMutated.Zones = zones

	nrtUpdated, err := cli.TopologyV1alpha1().NodeResourceTopologies(namespace).Update(context.TODO(), nrtMutated, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("Failed to update v1alpha1.NodeResourceTopology!:%v", err)
	}
	log.Printf("CRD instance updated resTopo: %v", nrtUpdated)
	return nil
}
