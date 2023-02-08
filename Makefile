COMMONENVVAR=GOOS=linux GOARCH=amd64
BUILDENVVAR=CGO_ENABLED=0
TOPOLOGYAPI_MANIFESTS=https://raw.githubusercontent.com/k8stopologyawareschedwg/noderesourcetopology-api/master/manifests

KUBECLI ?= kubectl
RUNTIME ?= podman
REPOOWNER ?= k8stopologyawarewg
IMAGENAME ?= resource-topology-exporter
IMAGETAG ?= latest
RTE_CONTAINER_IMAGE ?= quay.io/$(REPOOWNER)/$(IMAGENAME):$(IMAGETAG)


.PHONY: all
all: build extra-tools

.PHONY: build-tools
build-tools: outdir _out/git-semver

.PHONY: extra-tools
extra-tools: outdir _out/nrtstress

.PHONY: build
build: build-tools
	$(COMMONENVVAR) $(BUILDENVVAR) go build \
	-ldflags "-s -w -X github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/version.version=$(shell _out/git-semver)" \
	-o _out/resource-topology-exporter cmd/resource-topology-exporter/main.go

.PHONY: gofmt
gofmt:
	@echo "Running gofmt"
	gofmt -s -w `find . -path ./vendor -prune -o -type f -name '*.go' -print`

.PHONY: govet
govet:
	@echo "Running go vet"
	go vet

outdir:
	@mkdir -p _out || :

.PHONY: deps-update
deps-update:
	go mod tidy && go mod vendor

.PHONY: deps-clean
deps-clean:
	rm -rf vendor

.PHONY: binaries-all
binaries-all: outdir deps-update extra-tools build

.PHONY: binaries
binaries: outdir deps-update build

.PHONY: clean
clean:
	rm -rf _out

.PHONY: image
image: outdir build
	@echo "building image"
	$(RUNTIME) build -f images/Dockerfile -t $(RTE_CONTAINER_IMAGE) --build-arg VERSION=$(shell _out/git-semver) --build-arg GIT_COMMIT=$(shell git log -1 --format=%H) .

.PHONY: image-assemble
image-assemble:
	@echo "building image"
	$(RUNTIME) build -f images/Dockerfile -t $(RTE_CONTAINER_IMAGE) .

.PHONY: push
push: image
	@echo "pushing image"
	$(RUNTIME) push $(RTE_CONTAINER_IMAGE)

.PHONY: test-unit
test-unit:
	@go test ./cmd/...
	@go test ./pkg/...

.PHONY: build-e2e
build-e2e: _out/rte-e2e.test

_out/rte-e2e.test: outdir test/e2e/*.go test/e2e/utils/*.go
	go test -v -c -o _out/rte-e2e.test ./test/e2e/

.PHONY: test-e2e
test-e2e: build-e2e
	_out/rte-e2e.test -ginkgo.focus="RTE"

.PHONY: test-e2e-full
	go test -v ./test/e2e/

.PHONY: deploy
deploy:
	$(KUBECLI) create -f manifests/noderesourcetopologies_crd.yaml
	hack/get-manifests.sh | $(KUBECLI) create -f -

.PHONY: undeploy
undeploy:
	$(KUBECLI) delete -f manifests/noderesourcetopologies_crd.yaml
	hack/get-manifests.sh | $(KUBECLI) delete -f -

.PHONY: gen-manifests
gen-manifests:
	@cat manifests/noderesourcetopologies_crd.yaml
	@hack/get-manifests.sh

.PHONY: gen-manifests-polling
gen-manifests-polling:
	@cat manifests/noderesourcetopologies_crd.yaml
	@hack/get-manifests.sh polling

.PHONY: gen-manifests-evented
gen-manifests-evented:
	@cat manifests/noderesourcetopologies_crd.yaml
	@hack/get-manifests.sh evented

.PHONY: update-manifests
update-manifests:
	@curl -L $(TOPOLOGYAPI_MANIFESTS)/crd.yaml -o manifests/noderesourcetopologies_crd.yaml

.PHONY: update-golden-files
update-golden-files:
	@go test ./pkg/config/... -update

# helper tools
_out/nrtstress: outdir
	$(COMMONENVVAR) $(BUILDENVVAR) go build \
	-o _out/nrtstress tools/nrtstress/main.go

# build tools:
#
_out/git-semver: outdir
	@go build -o _out/git-semver vendor/github.com/mdomke/git-semver/main.go
