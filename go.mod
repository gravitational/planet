module github.com/gravitational/planet

go 1.13

require (
	github.com/alecthomas/units v0.0.0-20190924025748-f65c72e2690d // indirect
	github.com/aws/aws-sdk-go v1.25.41
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/cloudfoundry/gosigar v1.1.1-0.20180907192854-50ddd08d81d7 // indirect
	github.com/containerd/cgroups v1.0.3
	github.com/containerd/console v1.0.2
	github.com/containerd/containerd v1.5.18 // indirect
	github.com/coreos/go-systemd/v22 v22.3.2
	github.com/davecgh/go-spew v1.1.1
	github.com/docker/docker v0.0.0-00010101000000-000000000000
	github.com/docker/go-connections v0.4.0
	github.com/fatih/color v1.9.0
	github.com/ghodss/yaml v1.0.0
	github.com/godbus/dbus v4.1.0+incompatible // indirect
	github.com/godbus/dbus/v5 v5.0.4
	github.com/gravitational/configure v0.0.0-20180808141939-c3428bd84c23
	github.com/gravitational/coordinate v0.0.0-20210729105737-9889f283ee4f
	github.com/gravitational/coordinate/v4 v4.0.0
	github.com/gravitational/etcd-backup v0.0.0-20210730122523-5067e2c92759
	github.com/gravitational/satellite v0.0.9-0.20211001145055-fae4abf39d92
	github.com/gravitational/trace v1.1.11
	github.com/gravitational/version v0.0.2-0.20170324200323-95d33ece5ce1
	github.com/grpc-ecosystem/go-grpc-middleware v1.2.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.14.5 // indirect
	github.com/imdario/mergo v0.3.12
	github.com/jkeiser/iter v0.0.0-20200628201005-c8aa0ae784d1 // indirect
	github.com/jochenvg/go-udev v0.0.0-20171110120927-d6b62d56d37b
	github.com/jonboulle/clockwork v0.2.2 // indirect
	github.com/konsorten/go-windows-terminal-sequences v1.0.3 // indirect
	github.com/kylelemons/godebug v0.0.0-20170820004349-d65d576e9348
	github.com/magefile/mage v1.10.0
	github.com/mattn/go-colorable v0.1.6 // indirect
	github.com/mitchellh/go-ps v1.0.0
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/opencontainers/runc v1.0.2
	github.com/opencontainers/runtime-spec v1.0.3-0.20210326190908-1c3f411f0417
	github.com/opencontainers/selinux v1.8.2
	github.com/pborman/uuid v1.2.0
	github.com/prometheus/client_golang v1.7.1
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.7.0
	github.com/syndtr/gocapability v0.0.0-20200815063812-42c35b437635
	go.etcd.io/etcd v3.3.22+incompatible
	golang.org/x/sys v0.0.0-20220722155257-8c9f86f7a55f
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.0-20200605160147-a5ece683394c // indirect
	k8s.io/api v0.20.6
	k8s.io/apimachinery v0.20.6
	k8s.io/client-go v0.20.6
	k8s.io/kubelet v0.19.8
)

replace (
	github.com/alecthomas/units => github.com/alecthomas/units v0.0.0-20151022065526-2efee857e7cf
	github.com/coreos/go-systemd => github.com/coreos/go-systemd v0.0.0-20180202092358-40e2722dffea
	github.com/coreos/pkg => github.com/coreos/pkg v0.0.0-20180108230652-97fdf19511ea
	github.com/docker/docker => github.com/docker/engine v17.12.0-ce-rc1.0.20200204220554-5f6d6f3f2203+incompatible
	github.com/google/uuid => github.com/google/uuid v1.0.0
	github.com/json-iterator/go => github.com/json-iterator/go v1.1.5
	github.com/sirupsen/logrus => github.com/gravitational/logrus v1.4.3
	go.etcd.io/etcd => go.etcd.io/etcd v0.5.0-alpha.5.0.20200401174654-e694b7bb0875
	golang.org/x/oauth2 => golang.org/x/oauth2 v0.0.0-20181017192945-9dcd33a902f4
	golang.org/x/text => golang.org/x/text v0.3.0
	golang.org/x/time => golang.org/x/time v0.0.0-20180412165947-fbb02b2291d2
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20181016170114-94acd270e44e
	google.golang.org/grpc => google.golang.org/grpc v1.26.0
	gopkg.in/alecthomas/kingpin.v2 => github.com/gravitational/kingpin v2.1.11-0.20180808090833-85085db9f49b+incompatible
	gopkg.in/yaml.v2 => gopkg.in/yaml.v2 v2.2.2
)
