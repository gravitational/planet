FROM cube/base

ADD ./cube-master $BUILDDIR/cube-master
ADD ./cube-node $BUILDDIR/cube-node
ADD ./cube-registry $BUILDDIR/cube-registry

RUN ROOTFS=${BUILDDIR}/rootfs make -C $BUILDDIR/cube-master/etcd -f etcd.mk
RUN ROOTFS=${BUILDDIR}/rootfs make -C $BUILDDIR/cube-master/k8s-master -f k8s-master.mk
RUN ROOTFS=${BUILDDIR}/rootfs make -C $BUILDDIR/cube-node/k8s-node -f k8s-node.mk
RUN ROOTFS=${BUILDDIR}/rootfs make -C $BUILDDIR/cube-registry -f registry.mk

RUN cd $BUILDDIR && tar -czf kube-dev.tar.gz rootfs