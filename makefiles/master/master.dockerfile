FROM planet/base

ADD ./makefiles/master $BUILDDIR/master

RUN ROOTFS=${BUILDDIR}/aci/rootfs make -C $BUILDDIR/master/etcd -f etcd.mk
RUN ROOTFS=${BUILDDIR}/aci/rootfs make -C $BUILDDIR/master/k8s-master -f k8s-master.mk

COPY ./makefiles/orbit.manifest.json $BUILDDIR/aci/orbit.manifest.json
RUN cd $BUILDDIR && tar -czf planet-master.tar.gz -C aci .
