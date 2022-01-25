
KUBE_VER ?= v1.21.5
SECCOMP_VER ?= 2.3.1-2.1+deb9u1
DOCKER_VER ?= 20.10.7
# we currently use our own flannel fork: gravitational/flannel
FLANNEL_VER := v0.10.6-gravitational
HELM_VER := 2.16.12
HELM3_VER := 3.3.4
COREDNS_VER := 1.7.0
NODE_PROBLEM_DETECTOR_VER := v0.6.4
CNI_VER := 0.8.6
IPTABLES_VER := v1.8.5
BUILDBOX_GO_VER ?= 1.17.5
DISTRIBUTION_VER=v2.7.1-gravitational
# aws-encryption-provider repo does not currently provide tagged releases
AWS_ENCRYPTION_PROVIDER_VER := c4abcb30b4c1ab1961369e1e50a98da2cedb765d

# planet user to use inside the rootfs tarball. This serves as a placeholder
# and the files will be owned by the actual planet user after extraction
PLANET_UID ?= 980665
PLANET_GID ?= 980665

# ETCD Versions to include in the release
# This list needs to include every version of etcd that we can upgrade from + latest
# Version log
# v3.3.4
# v3.3.9  - 5.2.x,
# v3.3.11 - 5.5.x,
# v3.3.12 - 6.3.x, 6.1.x, 5.5.x
# v3.3.15 - 6.3.x
# v3.3.20 - 6.3.x, 6.1.x, 5.5.x
# v3.3.22 - 6.3.x, 6.1.x, 5.5.x
# v3.4.3  - 7.0.x
# v3.4.7  - 7.0.x
# v3.4.9   - 7.0.x
ETCD_VER := v3.3.12 v3.3.15 v3.3.20 v3.3.22 v3.4.3 v3.4.7 v3.4.9
# This is the version of etcd we should upgrade to (from the version list)
# Note: When bumping the ETCD_LATEST_VERSION, please ensure that:
#   - The version of etcd vendored as a library is the same (Gopkg.toml)
#   - Modify build.go and run the etcd upgrade integration test (go run mage.go ci:testEtcdUpgrade)
ETCD_LATEST_VER := v3.4.9
