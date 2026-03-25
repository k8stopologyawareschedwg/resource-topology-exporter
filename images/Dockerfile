FROM  quay.io/centos/centos:stream9 AS builder-pause
WORKDIR /go/src/rte
RUN dnf install -y make gcc
RUN dnf --enablerepo=crb install -y glibc-static
COPY . .
RUN make build-pause

FROM docker.io/golang:1.25 AS builder-rte
WORKDIR /go/src/rte
COPY . .
RUN make build

FROM  quay.io/centos/centos:stream9
RUN dnf install -y hwdata nc && dnf clean -y all
COPY --from=builder-rte /go/src/rte/_out/resource-topology-exporter /bin/resource-topology-exporter
COPY --from=builder-pause /go/src/rte/_out/pause /bin/pause
RUN mkdir -p /etc/rte /etc/secrets/rte/tls /etc/secrets/rte/ca

ARG GIT_COMMIT
ARG VERSION
RUN \
    # Check for mandatory build arguments
    : "${GIT_COMMIT:?Build argument needs to be set and non-empty.}" \
    && : "${VERSION:?Build argument needs to be set and non-empty.}"

LABEL org.opencontainers.image.revision=${GIT_COMMIT}
LABEL org.opencontainers.image.version=${VERSION}

ENTRYPOINT ["/bin/resource-topology-exporter"]
