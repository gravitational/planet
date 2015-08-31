FROM planet/base

ADD ./makefiles/master $BUILDDIR/master

RUN ROOTFS=${BUILDDIR}/aci/rootfs make -C $BUILDDIR/master/etcd -f etcd.mk
RUN ROOTFS=${BUILDDIR}/aci/rootfs make -C $BUILDDIR/master/k8s-master -f k8s-master.mk

COPY ./makefiles/aci.manifest $BUILDDIR/aci/manifest
RUN cd $BUILDDIR && tar -czf planet-master.aci -C aci .
RUN actool validate $BUILDDIR/planet-master.aci
