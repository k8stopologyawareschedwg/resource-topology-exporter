# resource-topology-exporter
Resource Topology exporter for Topology Aware Scheduler


This is resource topology exporter to enable NUMA-aware scheduling. We introduce a standalone daemon which runs on each node in the cluster as a daemonset. It collect resources allocated to running pods along with associated topology (NUMA nodes) and provides information of the available resources (with numa node granularity) through a CRD instance created per node.
so that the scheduler can use it to make a NUMA aware placement decision.


## Background
Currently scheduler is incapable of correctly accounting for the available resources and their associated topology information. Topology manager is responsible for identifying numa nodes on which the resources are allocated and scheduler is unaware of per-NUMA resource allocation.

A [KEP](https://github.com/AlexeyPerevalov/enhancements/blob/provisioning-resources-with-numa-topology/keps/sig-node/20200619-provisioning-resources-with-numa-topology.md) is currently in progress to expose per-NUMA node resource information to scheduler through CRD


## CRD

Available resources with topology of the node should be stored in CRD. Format of the topology described
[in this document](https://docs.google.com/document/d/12kj3fK8boNuPNqob6F_pPU9ZTaNEnPGaXEooW1Cilwg/edit).


```go
// NodeResourceTopology is a specification for a Foo resource
type NodeResourceTopology struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	TopologyPolicy []string `json:"topologyPolicies"`
	Zones          ZoneMap  `json:"zones"`
}

// Zone is the spec for a NodeResourceTopology resource
type Zone struct {
	Type       string          `json:"type"`
	Parent     string          `json:"parent,omitempty"`
	Costs      map[string]int  `json:"costs,omitempty"`
	Attributes map[string]int  `json:"attributes,omitempty"`
	Resources  ResourceInfoMap `json:"resources,omitempty"`
}

type ZoneMap map[string]Zone
type ResourceInfoMap map[string]ResourceInfo

type ResourceInfo struct {
	Allocatable string `json:"allocatable"`
	Capacity    string `json:"capacity"`
}

```
## Design based on Pod Resource API
Kubelet exposes endpoint at `/var/lib/kubelet/pod-resources/kubelet.sock` for exposing information about assignment of devices to containers. It obtains this information from the internal state of the kubelet's Device Manager and returns a single PodResourcesResponse enabling monitor applications to poll for resources allocated to pods and containers on the node. This makes PodResource API a reasonable way of obtaining allocated resource information.

However, [PodResource API](https://godoc.org/k8s.io/kubernetes/pkg/kubelet/apis/podresources/v1alpha1) currently only exposes devices as the container resources (without topology info). We are proposing [KEP](https://github.com/kubernetes/enhancements/pull/1884) to enhance it to expose CPU information along with device topology info.
In order to use pod-resource-api source in Resource Topology Exporter, you will need to use patched version of kubelet implementing the changes proposed in the aforementioned KEPs:
1. https://github.com/kubernetes/kubernetes/pull/93243/files
1. https://github.com/fromanirh/kubernetes/tree/podresources-get-available-devices

 A kubernetes branch with both these features that was used for testing is available [here](https://github.com/swatisehgal/kubernetes/tree/podResGetAvailResTopoInfoCpuId)

 This will no longer be needed once the KEP and the PR are merged.

Furthermore, changes are being proposed to enhance ([KEP](https://github.com/kubernetes/enhancements/pull/1926)) PodResource API to support a Watch() endpoint, enabling monitor applications to be notified of new resource allocation, release or resource allocation updates. This will be useful to enable Resource Topology Exporter to become more event based as opposed to its current mechanism of polling.

## Installation

1. You can use the following environment variables to configure the exporter image name:
   - `REPOOWNER`: name of the repository on which the image will be pushed (example: `quay.io/$REPOOWNER/...`)
   - `IMAGENAME`: name of the image to build
   - `IMAGETAG`: name of the image tag to use
2. To deploy the exporter run:

```bash
make push
make config
make deploy
```
The Makefile provides other targets:
* build: Build the device plugin go code
* gofmt: To format the code
* push: To push the docker image to a registry
* images: To build the docker image


## Workload requesting devices

To test the working of exporter, deploy test deployment that request resources
```bash
make deploy-pod
```

## Limitations

* RTE assumes the devices are not created dynamically.
* Due to the current (2020, Sept) limitations of CRI, we now rely on podresource API to obtain resource information. Details can be found in the alternatives section below. CRI support is available in the [release v0.1](https://github.com/k8stopologyawareschedwg/resource-topology-exporter/tree/v0.1) following which CRI support would be deprecated in this repository.



## Alternative Approach
### Design based on CRI
This daemon can also gather resource information using the Container Runtime interface.


The containerStatusResponse returned as a response to the ContainerStatus rpc contains `Info` field which is used by the container runtime for capturing ContainerInfo.
```go
message ContainerStatusResponse {
      ContainerStatus status = 1;
      map<string, string> info = 2;
}
```

Containerd has been used as the container runtime in the initial investigation. The internal container object info
[here](https://github.com/containerd/cri/blob/master/pkg/server/container_status.go#L130)

The Daemon set is responsible for the following:

- Parsing the info field to obtain container resource information
- Identifying NUMA nodes of the allocated resources
- Identifying total number of resources allocated on a NUMA node basis
- Detecting Node resource capacity on a NUMA node basis
- Updating the CRD instance per node indicating available resources on NUMA nodes, which is referred to the scheduler


### Drawbacks

The content of the `info` field is free form, unregulated by the API contract. So, CRI-compliant container runtime engines are not required to add any configuration-specific information, like for example cpu allocation, here. In case of containerd container runtime, the Linux Container Configuration is added in the `info` map depending on the verbosity setting of the container runtime engine.

There is currently work going on in the community as part of the the Vertical Pod Autoscaling feature to update the ContainerStatus field to report back containerResources
[KEP](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/20191025-kubelet-container-resources-cri-api-changes.md).
