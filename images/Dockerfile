FROM  quay.io/centos/centos:stream9
RUN dnf install -y hwdata nc && dnf clean -y all
ADD _out/resource-topology-exporter /bin/resource-topology-exporter
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
