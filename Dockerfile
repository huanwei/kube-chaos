FROM centos:7.2.1511

RUN yum install -y iproute \
 && yum clean all

COPY kube-chaos /usr/local/bin/
COPY etcdctl  /usr/local/bin/

ENTRYPOINT ["kube-chaos"]


# GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -v -i -o kube-chaos  kube-chaos.go