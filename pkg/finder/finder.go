package finder

import (
	"k8s.io/api/core/v1"
	"time"
)

type Args struct {
	Source                string
	PodResourceSocketPath string
	SleepInterval         time.Duration
	Namespace             string
	SysfsRoot             string
	KubeletConfigFile     string
}

type ResourceInfo struct {
	Name v1.ResourceName
	Data []string
}

type ContainerResources struct {
	Name      string
	Resources []ResourceInfo
}

type PodResources struct {
	Name       string
	Namespace  string
	Containers []ContainerResources
}

type Finder interface {
	Scan(map[string]string) ([]PodResources, error)
}
