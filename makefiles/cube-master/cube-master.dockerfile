FROM cube/base

ADD ./makefiles/cube-master $BUILDDIR/cube-master

RUN ROOTFS=${BUILDDIR}/aci/rootfs make -C $BUILDDIR/cube-master/etcd -f etcd.mk
RUN ROOTFS=${BUILDDIR}/aci/rootfs make -C $BUILDDIR/cube-master/k8s-master -f k8s-master.mk

COPY ./makefiles/aci.manifest $BUILDDIR/aci/manifest
RUN cd $BUILDDIR && tar -czf cube-master.aci -C aci .
RUN actool validate $BUILDDIR/cube-master.aci