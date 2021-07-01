module github.com/gravitational/planet

go 1.16

require (
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/Microsoft/go-winio v0.4.14 // indirect
	github.com/cenkalti/backoff v2.0.0+incompatible
	github.com/checkpoint-restore/go-criu v0.0.0-20181120144056-17b0214f6c48 // indirect
	github.com/cilium/ebpf v0.0.0-20200224172853-0b019ed01187 // indirect
	github.com/cloudfoundry/gosigar v1.1.1-0.20180907192854-50ddd08d81d7 // indirect
	github.com/containerd/cgroups v0.0.0-20181219155423-39b18af02c41
	github.com/containerd/console v0.0.0-20180307192801-cb7008ab3d83
	github.com/containerd/containerd v1.3.3 // indirect
	github.com/coreos/etcd v3.3.25+incompatible // indirect
	github.com/coreos/go-systemd v0.0.0-20180511133405-39ca1b05acc7
	github.com/coreos/pkg v0.0.0-20180108230652-97fdf19511ea // indirect
	github.com/cyphar/filepath-securejoin v0.2.2 // indirect
	github.com/davecgh/go-spew v1.1.1
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/docker v0.0.0-00010101000000-000000000000
	github.com/docker/go-connections v0.4.0
	github.com/docker/go-units v0.3.3 // indirect
	github.com/fatih/color v1.9.0
	github.com/ghodss/yaml v1.0.0
	github.com/godbus/dbus v4.1.0+incompatible
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/gravitational/configure v0.0.0-20180808141939-c3428bd84c23
	github.com/gravitational/coordinate v0.0.0-20200227044100-12af3c0f9593
	github.com/gravitational/etcd-backup v0.0.0-20201012185408-87328521981c
	github.com/gravitational/go-udev v0.0.0-20160615210516-4cc8baba3689
	github.com/gravitational/satellite v0.0.9-0.20201119181211-aef8c3a377eb
	github.com/gravitational/trace v1.1.11
	github.com/gravitational/version v0.0.2-0.20170324200323-95d33ece5ce1
	github.com/hashicorp/serf v0.9.5
	github.com/imdario/mergo v0.3.6
	github.com/kylelemons/godebug v0.0.0-20170820004349-d65d576e9348
	github.com/magefile/mage v1.9.0
	github.com/mitchellh/go-ps v1.0.0
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/mrunalp/fileutils v0.0.0-20171103030105-7d4729fb3618 // indirect
	github.com/opencontainers/go-digest v1.0.0-rc1 // indirect
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/opencontainers/runc v1.0.0-rc10
	github.com/opencontainers/runtime-spec v1.0.2-0.20190207185410-29686dbc5559
	github.com/opencontainers/selinux v1.4.0
	github.com/pborman/uuid v0.0.0-20180906182336-adf5a7427709
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_golang v1.4.0
	github.com/seccomp/libseccomp-golang v0.9.0 // indirect
	github.com/sirupsen/logrus v1.7.0
	github.com/stretchr/testify v1.6.1
	github.com/syndtr/gocapability v0.0.0-20180223013746-33e07d32887e
	github.com/vishvananda/netlink v1.0.0 // indirect
	github.com/vishvananda/netns v0.0.0-20180720170159-13995c7128cc // indirect
	go.etcd.io/bbolt v1.3.4 // indirect
	go.etcd.io/etcd v3.3.22+incompatible
	go.uber.org/multierr v1.5.0 // indirect
	go.uber.org/zap v1.14.0 // indirect
	golang.org/x/lint v0.0.0-20200130185559-910be7a94367 // indirect
	golang.org/x/sys v0.0.0-20210616094352-59db8d763f22
	golang.org/x/tools v0.0.0-20200225230052-807dcd883420 // indirect
	google.golang.org/grpc v1.26.0 // indirect
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15
	gopkg.in/yaml.v2 v2.2.8
	gotest.tools v2.2.0+incompatible // indirect
	k8s.io/api v0.17.3
	k8s.io/apimachinery v0.17.5-beta.0
	k8s.io/client-go v0.17.3
	k8s.io/kubelet v0.17.4-0.20200212000225-d73154502cda
)

replace (
	github.com/docker/docker => github.com/docker/engine v17.12.0-ce-rc1.0.20200204220554-5f6d6f3f2203+incompatible
	github.com/gravitational/satellite => github.com/a-palchikov/satellite v0.0.9-0.20210701113341-c00eafc55855
	github.com/sirupsen/logrus => github.com/gravitational/logrus v1.4.3
	go.etcd.io/etcd => go.etcd.io/etcd v0.5.0-alpha.5.0.20200401174654-e694b7bb0875
	gopkg.in/alecthomas/kingpin.v2 => github.com/gravitational/kingpin v2.1.11-0.20180808090833-85085db9f49b+incompatible
	k8s.io/kubelet => k8s.io/kubelet v0.17.3
)
