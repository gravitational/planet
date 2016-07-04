FROM alpine:3.4

MAINTAINER "Gravitational" <admin@gravitational.com>

ADD hook.dockerfile /Dockerfile

ARG KUBE_VER
# do not install ca-certificates since wget is unable to verify google's certificate for storage.googleapis.com
RUN apk add --no-cache --update wget bash && \
	wget --no-check-certificate https://storage.googleapis.com/kubernetes-release/release/${KUBE_VER}/bin/linux/amd64/kubectl \
		-P /usr/local/bin/ && \
	chmod +x /usr/local/bin/kubectl && \
	apk del wget && \
	rm -rfv /var/cache/apk/*

ENTRYPOINT ["/usr/local/bin/kubectl"]
