FROM cube/base

ADD ./makefiles/cube-master $BUILDDIR/cube-master
ADD ./makefiles/cube-node $BUILDDIR/cube-node

RUN ROOTFS=${BUILDDIR}/aci/rootfs make -C $BUILDDIR/cube-master/etcd -f etcd.mk
RUN ROOTFS=${BUILDDIR}/aci/rootfs make -C $BUILDDIR/cube-master/k8s-master -f k8s-master.mk
RUN ROOTFS=${BUILDDIR}/aci/rootfs make -C $BUILDDIR/cube-node/k8s-node -f k8s-node.mk

COPY ./makefiles/aci.manifest $BUILDDIR/aci/manifest
RUN cd $BUILDDIR && tar -czf cube-dev.aci -C aci .
RUN actool validate $BUILDDIR/cube-dev.aci

