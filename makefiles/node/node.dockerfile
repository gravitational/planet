FROM planet/base

ADD ./makefiles/node $BUILDDIR/node

RUN ROOTFS=${BUILDDIR}/aci/rootfs make -C $BUILDDIR/node/etcdctl -f etcdctl.mk
RUN ROOTFS=${BUILDDIR}/aci/rootfs make -C $BUILDDIR/node/k8s-node -f k8s-node.mk

COPY ./makefiles/aci.manifest $BUILDDIR/aci/manifest
RUN cd $BUILDDIR && tar -czf planet-node.aci -C aci .
RUN actool validate $BUILDDIR/planet-node.aci
