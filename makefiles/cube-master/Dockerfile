FROM cube/base

ADD . $BUILDDIR/cube-master

RUN ROOTFS=${BUILDDIR}/rootfs make -C $BUILDDIR/cube-master/etcd -f etcd.mk
RUN ROOTFS=${BUILDDIR}/rootfs make -C $BUILDDIR/cube-master/k8s-master -f k8s-master.mk

RUN cd $BUILDDIR && tar -czf kube-master.tar.gz rootfs