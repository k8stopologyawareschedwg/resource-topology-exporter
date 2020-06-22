COMMONENVVAR=GOOS=linux GOARCH=amd64
BUILDENVVAR=CGO_ENABLED=0

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

.PHONY: image
image: build
	@echo "building image"
	docker build -f images/Dockerfile -t quay.io/swsehgal/resource-topology-exporter:latest .

.PHONY: push
push: image
	@echo "pushing image"
	docker push quay.io/swsehgal/resource-topology-exporter:latest

.PHONY: deploy
deploy: push
	@echo "deploying Resource Topology Exporter"
	kubectl create -f manifests/resource-topology-exporter-ds.yaml

.PHONY: deploy-pod
deploy-pod:
	@echo "deploying Guaranteed Pod"
	kubectl create -f manifests/test-deployment.yaml
	kubectl create -f manifests/test-deployment-2.yaml

.PHONY: deploy-taerror
deploy-taerror:
	@echo "deploying Pod"
	kubectl create -f manifests/test-deployment-taerror.yaml

clean-binaries:
	rm -f bin/resource-topology-exporter
	
clean: clean-binaries
	kubectl delete -f manifests/resource-topology-exporter-ds.yaml
	kubectl delete -f manifests/test-deployment.yaml
	kubectl delete -f manifests/test-deployment-2.yaml
