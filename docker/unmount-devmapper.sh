#!/bin/bash

# Cleanup any stale mounts left from previous shutdown
# see https://bugs.launchpad.net/ubuntu/+source/docker.io/+bug/1404300
grep "mapper/docker" /proc/mounts | /usr/bin/awk '{ print $2 }' | xargs -r umount || true
