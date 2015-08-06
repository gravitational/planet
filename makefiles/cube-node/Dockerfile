FROM cube/base

ADD . $BUILDDIR/cube-node

RUN ROOTFS=${BUILDDIR}/rootfs make -C $BUILDDIR/cube-node/etcdctl -f etcdctl.mk
RUN ROOTFS=${BUILDDIR}/rootfs make -C $BUILDDIR/cube-node/k8s-node -f k8s-node.mk

RUN cd $BUILDDIR && tar -czf kube-node.tar.gz rootfs
