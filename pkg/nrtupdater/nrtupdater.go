package nrtupdater

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

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

	nrt, err := cli.TopologyV1alpha1().NodeResourceTopologies(te.args.Namespace).Get(context.TODO(), te.args.Hostname, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		nrtNew := v1alpha1.NodeResourceTopology{
			ObjectMeta: metav1.ObjectMeta{
				Name: te.args.Hostname,
			},
			Zones:            zones,
			TopologyPolicies: []string{te.tmPolicy},
		}

		nrtCreated, err := cli.TopologyV1alpha1().NodeResourceTopologies(te.args.Namespace).Create(context.TODO(), &nrtNew, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("update failed to create v1alpha1.NodeResourceTopology!:%v", err)
		}
		log.Printf("update created CRD instance: %v", dumpobject.DumpObject(nrtCreated))
		return nil
	}

	if err != nil {
		return err
	}

	nrtMutated := nrt.DeepCopy()
	nrtMutated.Zones = zones

	nrtUpdated, err := cli.TopologyV1alpha1().NodeResourceTopologies(te.args.Namespace).Update(context.TODO(), nrtMutated, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update failed to update v1alpha1.NodeResourceTopology!:%v", err)
	}
	log.Printf("update changed CRD instance: %v", nrtUpdated)
	return nil
}

func (te *NRTUpdater) Run(zonesChannel <-chan v1alpha1.ZoneList) chan<- struct{} {
	done := make(chan struct{})
	go func() {
		for {
			select {
			case zonesValue := <-zonesChannel:
				tsBegin := time.Now()
				if err := te.Update(zonesValue); err != nil {
					log.Printf("failed to update: %v", err)
				}
				tsEnd := time.Now()

				log.Printf("update request received at %v completed in %v", tsBegin, tsEnd.Sub(tsBegin))
				if te.args.Oneshot {
					break
				}
			case <-done:
				log.Printf("update stop at %v", time.Now())
				break
			}
		}
	}()
	return done
}
