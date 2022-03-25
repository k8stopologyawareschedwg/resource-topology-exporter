FROM  quay.io/centos/centos:centos7.9.2009
RUN yum install -y hwdata && yum clean -y all
ADD _out/resource-topology-exporter /bin/resource-topology-exporter
ENTRYPOINT ["/bin/resource-topology-exporter"]
