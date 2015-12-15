#!/bin/bash

# -----------------------------------------------------------------------------
# Version management helpers - mostly verbatim copy from kubernetes.  These functions
# help set the following variables:
#
#    GIT_COMMIT - The git commit id corresponding to this source code.
#    GIT_TREE_STATE - 
#         "clean" indicates no changes since the git commit id
#         "dirty" indicates source code changes after the git commit id
#    VERSION - "vX.Y" used to indicate the last release version.

ROOT=$(pwd)
GO_PACKAGE=github.com/gravitational/planet

get_version_vars() {
  local git=(git --work-tree "${ROOT}")

  if [[ -n ${GIT_COMMIT-} ]] || GIT_COMMIT=$("${git[@]}" rev-parse "HEAD^{commit}" 2>/dev/null); then
    if [[ -z ${GIT_TREE_STATE-} ]]; then
      # Check if the tree is dirty.  default to dirty
      if git_status=$("${git[@]}" status --porcelain 2>/dev/null) && [[ -z ${git_status} ]]; then
        GIT_TREE_STATE="clean"
      else
        GIT_TREE_STATE="dirty"
      fi
    fi

    # Use git describe to find the version based on annotated tags.
    if [[ -n ${VERSION-} ]] || VERSION=$("${git[@]}" describe --tags --abbrev=14 "${GIT_COMMIT}^{commit}" 2>/dev/null); then
      # This translates the "git describe" to an actual semver.org
      # compatible semantic version that looks something like this:
      #   v1.1.0-alpha.0.6+84c76d1142ea4d
      #
      VERSION=$(echo "${VERSION}" | sed "s/-\([0-9]\{1,\}\)-g\([0-9a-f]\{14\}\)$/.\1\+\2/")
      if [[ "${GIT_TREE_STATE}" == "dirty" ]]; then
        # git describe --dirty only considers changes to existing files, but
        # that is problematic since new untracked .go files affect the build,
        # so use our idea of "dirty" from git status instead.
        VERSION+="-dirty"
      fi
    fi
  fi
}

# golang 1.5 wants `-X key=val`, but golang 1.4- REQUIRES `-X key val`
ldflag() {
  local key=${1}
  local val=${2}

  GO_VERSION=($(go version))

  if [[ -z $(echo "${GO_VERSION[2]}" | grep -E 'go1.5') ]]; then
    echo "-X ${GO_PACKAGE}/lib/version.${key} ${val}"
  else
    echo "-X ${GO_PACKAGE}/lib/version.${key}=${val}"
  fi
}

# Prints the value that needs to be passed to the -ldflags parameter of go build
# in order to set the Kubernetes based on the git tree status.
ldflags() {
  get_version_vars

  local -a ldflags=()
  if [[ -n ${GIT_COMMIT-} ]]; then
    ldflags+=($(ldflag "gitCommit" "${GIT_COMMIT}"))
    ldflags+=($(ldflag "gitTreeState" "${GIT_TREE_STATE}"))
  fi

  if [[ -n ${VERSION-} ]]; then
    ldflags+=($(ldflag "version" "${VERSION}"))
  fi

  # The -ldflags parameter takes a single string, so join the output.
  echo "${ldflags[*]-}"
}

echo $(ldflags)
