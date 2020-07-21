COMMONENVVAR=GOOS=linux GOARCH=amd64
BUILDENVVAR=CGO_ENABLED=0

RUNTIME ?= podman
REPOOWNER ?= swsehgal
IMAGENAME ?= resource-topology-exporter
IMAGETAG ?= latest

.PHONY: all
all: build

.PHONY: build
build:
	$(COMMONENVVAR) $(BUILDENVVAR) go build -ldflags '-w' -o bin/resource-topology-exporter main.go

.PHONY: gofmt
gofmt:
	@echo "Running gofmt"
	gofmt -s -w `find . -path ./vendor -prune -o -type f -name '*.go' -print`

.PHONY: govet
govet:
	@echo "Running go vet"
	go vet

.PHONY: config
config:
	@echo "deploying configmap"
	kubectl create -f config/examples/sriovdp-configmap.yaml

.PHONY: image
image: build
	@echo "building image"
	$(RUNTIME) build -f images/Dockerfile -t quay.io/$(REPOOWNER)/$(IMAGENAME):$(IMAGETAG) .

.PHONY: crd
crd:
	@echo "deploying crd"
	kubectl create -f manifests/crd-v1alpha1.yaml

.PHONY: push
push: image
	@echo "pushing image"
	$(RUNTIME) push quay.io/$(REPOOWNER)/$(IMAGENAME):$(IMAGETAG)

.PHONY: deploy
deploy: push
	@echo "deploying Resource Topology Exporter"
	kubectl create -f manifests/resource-topology-exporter-ds.yaml

.PHONY: deploy-pod
deploy-pod:
	@echo "deploying Guaranteed Pod"
	kubectl create -f manifests/test-sriov-pod.yaml
	kubectl create -f manifests/test-sriov-pod-2.yaml
	kubectl create -f manifests/test-sriov-pod-3.yaml

.PHONY: deploy-taerror
deploy-taerror:
	@echo "deploying Pod"
	kubectl create -f manifests/test-deployment-taerror.yaml

clean-binaries:
	rm -f bin/resource-topology-exporter

clean: clean-binaries
	kubectl delete -f manifests/resource-topology-exporter-ds.yaml
	kubectl delete -f manifests/test-sriov-pod.yaml
	kubectl delete -f manifests/test-sriov-pod-2.yaml
	kubectl delete -f manifests/test-sriov-pod-3.yaml
