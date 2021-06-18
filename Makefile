COMMONENVVAR=GOOS=linux GOARCH=amd64
BUILDENVVAR=CGO_ENABLED=0
TOPOLOGYAPI_MANIFESTS=https://raw.githubusercontent.com/k8stopologyawareschedwg/noderesourcetopology-api/master/manifests

KUBECLI ?= kubectl
RUNTIME ?= podman
REPOOWNER ?= k8stopologyawarewg
IMAGENAME ?= resource-topology-exporter
IMAGETAG ?= latest

.PHONY: all
all: build

.PHONY: build
build: outdir
	$(COMMONENVVAR) $(BUILDENVVAR) go build -ldflags '-w' -o _out/resource-topology-exporter cmd/resource-topology-exporter/main.go

.PHONY: gofmt
gofmt:
	@echo "Running gofmt"
	gofmt -s -w `find . -path ./vendor -prune -o -type f -name '*.go' -print`

.PHONY: govet
govet:
	@echo "Running go vet"
	go vet

outdir:
	mkdir -p _out || :

.PHONY: deps-update
deps-update:
	go mod tidy && go mod vendor

.PHONY: deps-clean
deps-clean:
	rm -rf vendor

.PHONY: binaries
binaries: outdir deps-update build

.PHONY: clean
clean:
	rm -rf _out

.PHONY: image
image: binaries
	@echo "building image"
	$(RUNTIME) build -f images/Dockerfile -t quay.io/$(REPOOWNER)/$(IMAGENAME):$(IMAGETAG) .

.PHONY: push
push: image
	@echo "pushing image"
	$(RUNTIME) push quay.io/$(REPOOWNER)/$(IMAGENAME):$(IMAGETAG)

.PHONY: test-unit
test-unit:
	go test ./pkg/...

.PHONY: test-e2e
test-e2e: binaries
	ginkgo test/e2e

.PHONY: deploy
deploy:
	$(KUBECLI) create -f $(TOPOLOGYAPI_MANIFESTS)/crd.yaml
	hack/get-manifest-ds.sh | $(KUBECLI) create -f -

.PHONY: undeploy
undeploy:
	$(KUBECLI) delete -f $(TOPOLOGYAPI_MANIFESTS)/crd.yaml
	hack/get-manifest-ds.sh | $(KUBECLI) delete -f -

.PHONY: gen-manifests
gen-manifests:
	@curl -L $(TOPOLOGYAPI_MANIFESTS)/crd.yaml
	@hack/get-manifest-ds.sh
