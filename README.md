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

       TopologyPolicy string
       Nodes   []NUMANodeResource   `json:"nodes"`
}

// NUMANodeResource is the spec for a NodeResourceTopology resource
type NUMANodeResource struct {
       NUMAID int
       Resources v1.ResourceList
}
```
## Design based on CRI
This daemon gathers resource information using the Container Runtime interface.


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

NOTE
This exporter assumes the cluster has devices configured using [Sample device plugin](https://github.com/swatisehgal/sample-device-plugin)

## Limitations

Due to the current (2020, July) limitations of both CRI and podresources APIs for the purposes of the resource-topology-exporter,
we have explicit support for most commonly used device plugins.

### SRIOV device plugin
0. RTE - and SRIOV device plugin - always expect the PF device to be listed in the configuration. VFs are detected automatically.
1. the device capacity is calculated assuming VFs are created ahead of time; IOW RTE assumes the VFs are not created dynamically.
