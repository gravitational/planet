FROM cube/os

ADD . $GOPATH/src/github.com/gravitational/cube
RUN go install github.com/gravitational/cube/cube

ADD ./makefiles/ $BUILDDIR/makefiles

RUN ROOTFS=${BUILDDIR}/aci/rootfs make -C $BUILDDIR/makefiles/cube-base/network -f network.mk
RUN ROOTFS=${BUILDDIR}/aci/rootfs make -C $BUILDDIR/makefiles/cube-base/docker -f docker.mk 
RUN ROOTFS=${BUILDDIR}/aci/rootfs make -C $BUILDDIR/makefiles/cube-registry -f registry.mk

RUN mkdir -p ${BUILDDIR}/aci/rootfs/usr/bin && cp $GOPATH/bin/cube ${BUILDDIR}/aci/rootfs/usr/bin && chown 755 ${BUILDDIR}/aci/rootfs/usr/bin/cube
