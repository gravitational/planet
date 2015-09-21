FROM planet/os

ADD . $GOPATH/src/github.com/gravitational/planet
RUN go install github.com/gravitational/planet/tool/planet

ADD ./makefiles/ $BUILDDIR/makefiles

RUN ROOTFS=${BUILDDIR}/aci/rootfs make -C $BUILDDIR/makefiles/base/network -f network.mk
RUN ROOTFS=${BUILDDIR}/aci/rootfs make -C $BUILDDIR/makefiles/base/docker -f docker.mk 
RUN ROOTFS=${BUILDDIR}/aci/rootfs make -C $BUILDDIR/makefiles/registry -f registry.mk
RUN ROOTFS=${BUILDDIR}/aci/rootfs make -C $BUILDDIR/makefiles/kubernetes -f kubernetes.mk

RUN mkdir -p ${BUILDDIR}/aci/rootfs/usr/bin && cp $GOPATH/bin/planet ${BUILDDIR}/aci/rootfs/usr/bin && chown 755 ${BUILDDIR}/aci/rootfs/usr/bin/planet
