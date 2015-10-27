#!/bin/bash

# This script expects the following environment variables:
# KUBE_HOME   - directory with kubernetes repository as some test read configuration files
# TEST_HOME   - directory with test binaries and kubeconfig

if [[ -z "$KUBE_HOME" ]]; then
  echo -e "\033[1;31mExpected \$KUBE_HOME pointing to kubernetes repository\033[0m"
  exit 1
fi

if [[ -z "$TEST_HOME" ]]; then
  echo -e "\033[1;31mExpected \$TEST_HOME pointing to test assets directory\033[0m"
  exit 1
fi

e2e_test="$TEST_HOME/e2e.test"
ginkgo="$TEST_HOME/ginkgo"
planet_kube_master=$KUBE_MASTER

ginkgo_args=()
ginkgo_args+=("-p")           # emit progress info
ginkgo_args+=("-succinct")    # succinct reports
ginkgo_args+=("-v")           # verbose mode
ginkgo_args+=("-trace")       # print stack trace for failure
ginkgo_args+=("-failFast")    # stop after first failure
ginkgo_args+=("-focus=Networking") # FIXME: define which test(s) to run

"${ginkgo}" "${ginkgo_args[@]:+${ginkgo_args[@]}}" "${e2e_test}" -- \
  --host=${planet_kube_master} \
  --provider=planet \
  --num-nodes=2 \
  --kubeconfig="$KUBE_CONFIG" \
  --repo-root="$KUBE_HOME" \
  "${@:-}"
