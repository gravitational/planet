FROM planet/base

ADD ./makefiles/node $BUILDDIR/node

RUN ROOTFS=${BUILDDIR}/aci/rootfs make -C $BUILDDIR/node/etcdctl -f etcdctl.mk
RUN ROOTFS=${BUILDDIR}/aci/rootfs make -C $BUILDDIR/node/k8s-node -f k8s-node.mk

COPY ./makefiles/orbit.manifest.json $BUILDDIR/aci/orbit.manifest.json
RUN cd $BUILDDIR && tar -czf planet-node.tar.gz -C aci .
