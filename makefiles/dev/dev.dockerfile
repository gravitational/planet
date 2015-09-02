FROM planet/base

ADD ./makefiles/master $BUILDDIR/master
ADD ./makefiles/node $BUILDDIR/node

RUN ROOTFS=${BUILDDIR}/aci/rootfs make -C $BUILDDIR/master/etcd -f etcd.mk
RUN ROOTFS=${BUILDDIR}/aci/rootfs make -C $BUILDDIR/master/k8s-master -f k8s-master.mk
RUN ROOTFS=${BUILDDIR}/aci/rootfs make -C $BUILDDIR/node/k8s-node -f k8s-node.mk

COPY ./makefiles/orbit.manifest.json $BUILDDIR/aci/orbit.manifest.json
RUN cd $BUILDDIR && tar -czf planet-dev.tar.gz -C aci .
