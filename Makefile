COMMONENVVAR=GOOS=linux GOARCH=amd64
BUILDENVVAR=CGO_ENABLED=0
TOPOLOGYAPI_MANIFESTS=https://raw.githubusercontent.com/k8stopologyawareschedwg/noderesourcetopology-api/master/manifests
BINDIR=_out

KUBECLI ?= kubectl
RUNTIME ?= podman
REPOOWNER ?= k8stopologyawarewg
IMAGENAME ?= resource-topology-exporter
IMAGETAG ?= latest
RTE_CONTAINER_IMAGE ?= quay.io/$(REPOOWNER)/$(IMAGENAME):$(IMAGETAG)

GOLANGCI_LINT_VERSION=1.54.2
GOLANGCI_LINT_BIN=$(BINDIR)/golangci-lint
GOLANGCI_LINT_VERSION_TAG=v${GOLANGCI_LINT_VERSION}

KUBELET_MOD_VERSION = $(shell go list -m -f '{{ .Version }}' k8s.io/kubelet)

.PHONY: all
all: build

.PHONY: build-tools
build-tools: outdir _out/git-semver

.PHONY: extra-tools
extra-tools: outdir _out/nrtstress

.PHONY: build
build: build-tools
	$(COMMONENVVAR) $(BUILDENVVAR) go build \
	-ldflags "-s -w -X github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/version.version=$(shell _out/git-semver)" \
	-o _out/resource-topology-exporter cmd/resource-topology-exporter/main.go

.PHONY: build-dbg
build-dbg: build-tools
	$(COMMONENVVAR) $(BUILDENVVAR) go build \
	-ldflags "-X github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/version.version=$(shell _out/git-semver)" \
	-o _out/resource-topology-exporter cmd/resource-topology-exporter/main.go

.PHONY: gofmt
gofmt:
	@echo "Running gofmt"
	gofmt -s -w `find . -type f -name '*.go' -print`

.PHONY: govet
govet:
	@echo "Running go vet"
	go vet

outdir:
	@mkdir -p _out || :

.PHONY: deps-update
deps-update:
	go mod tidy

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

.PHONY: push
push: image
	@echo "pushing image"
	$(RUNTIME) push $(RTE_CONTAINER_IMAGE)

.PHONY: test-unit
test-unit:
	@go test ./cmd/...
	@go test ./pkg/...

# this is meant for developers only.
# DO NOT WIRE THIS IN CI! Let's use https://golangci-lint.run/usage/install/#github-actions instead
.PHONY: dev-lint
dev-lint: _out/golangci-lint
	$(GOLANGCI_LINT_BIN) run

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
_out/golangci-lint: outdir
	@if [ ! -x "$(GOLANGCI_LINT_BIN)" ]; then\
		echo "Downloading golangci-lint $(GOLANGCI_LINT_VERSION)";\
		curl -JL https://github.com/golangci/golangci-lint/releases/download/$(GOLANGCI_LINT_VERSION_TAG)/golangci-lint-$(GOLANGCI_LINT_VERSION)-linux-amd64.tar.gz -o _out/golangci-lint-$(GOLANGCI_LINT_VERSION)-linux-amd64.tar.gz;\
		tar xz -C _out -f _out/golangci-lint-$(GOLANGCI_LINT_VERSION)-linux-amd64.tar.gz;\
		ln -sf golangci-lint-1.54.2-linux-amd64/golangci-lint _out/golangci-lint;\
	else\
		echo "Using golangci-lint cached at $(GOLANGCI_LINT_BIN)";\
	fi

_out/git-semver: outdir
	@GOBIN=$(shell pwd)/_out go install github.com/mdomke/git-semver/v6@v6.9.0

_out/mockery: outdir
	@GOBIN=$(shell pwd)/_out go install github.com/vektra/mockery/v2@v2.43.1

gen-mocks: deps-update _out/mockery
	_out/mockery \
		--dir=$(GOPATH)/pkg/mod/k8s.io/kubelet@$(KUBELET_MOD_VERSION)/pkg/apis/podresources/v1 \
		--name=PodResourcesListerClient \
		--structname=MockPodResourcesListerClient \
		--filename=mock_PodResourcesListerClient.go \
		--output=pkg/podres \
		--outpkg=podres
