FROM cube/base

ADD ./makefiles/cube-node $BUILDDIR/cube-node

RUN ROOTFS=${BUILDDIR}/aci/rootfs make -C $BUILDDIR/cube-node/etcdctl -f etcdctl.mk
RUN ROOTFS=${BUILDDIR}/aci/rootfs make -C $BUILDDIR/cube-node/k8s-node -f k8s-node.mk

COPY ./makefiles/aci.manifest $BUILDDIR/aci/manifest
RUN cd $BUILDDIR && tar -czf cube-node.aci -C aci .
RUN actool validate $BUILDDIR/cube-node.aci
