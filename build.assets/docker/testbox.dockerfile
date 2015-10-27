FROM planet/buildbox

ENV GOPATH /gopath
ENV GOROOT /opt/go
ENV PATH $PATH:$GOPATH/bin:$GOROOT/bin
ENV KUBE_VER e3188f6ee7007000c5daf525c8cc32b4c5bf4ba8

# Install Kubernetes:
RUN mkdir -p $GOPATH/src/github.com/kubernetes; \
	cd $GOPATH/src/github.com/kubernetes; \
	git clone https://github.com/kubernetes/kubernetes && cd kubernetes && git checkout $KUBE_VER
