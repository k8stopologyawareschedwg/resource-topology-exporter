FROM  quay.io/centos/centos:8
RUN cd /etc/yum.repos.d/ && \
	sed -i 's/mirrorlist/#mirrorlist/g' /etc/yum.repos.d/CentOS-* && \
	sed -i 's|#baseurl=http://mirror.centos.org|baseurl=http://vault.centos.org|g' /etc/yum.repos.d/CentOS-*
RUN yum install -y hwdata && yum clean -y all
ADD _out/resource-topology-exporter /bin/resource-topology-exporter

ARG GIT_COMMIT
ARG VERSION
RUN \
    # Check for mandatory build arguments
    : "${GIT_COMMIT:?Build argument needs to be set and non-empty.}" \
    && : "${VERSION:?Build argument needs to be set and non-empty.}"

LABEL org.opencontainers.image.revision=${GIT_COMMIT}
LABEL org.opencontainers.image.version=${VERSION}

ENTRYPOINT ["/bin/resource-topology-exporter"]
