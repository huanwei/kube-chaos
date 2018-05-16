FROM alpine:3.6

MAINTAINER huanwei <huan@harmonycloud.com>

ENV GOPATH /go
ENV PATH $GOPATH/bin:/usr/local/go/bin:$PATH
RUN mkdir -p "$GOPATH/src" "$GOPATH/bin" && chmod -R 777 "$GOPATH"

# Add source files.
ADD *.go /go/src/github.com/huanwei/kube-chaos/
ADD pkg /go/src/github.com/huanwei/kube-chaos/pkg
ADD vendor /go/src/github.com/huanwei/kube-chaos/vendor

RUN set -ex \
	&& apk update && apk add --no-cache --virtual .build-deps \
		bash \
		musl-dev \
		openssl \
		go \
		ca-certificates \
    && cd /go/src/github.com/huanwei/kube-chaos \
    && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -v -i -o /bin/kube-chaos  kube-chaos.go \
	&& rm -rf /go \
	&& apk del .build-deps

CMD ["kube-chaos"]