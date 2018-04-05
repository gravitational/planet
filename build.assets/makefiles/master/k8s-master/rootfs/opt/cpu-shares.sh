#!/bin/sh
#
# This is a hack, to reset the /kubepods cpu.shares to be best effort until
# https://github.com/gravitational/gravity/issues/3241 can be addressed

while ! test -d /sys/fs/cgroup/cpu/kubepods/
do
    echo "Waiting for cgroup /sys/fs/cgroup/cpu/kubepods/..."
    (($cnt++)) && (($cnt==30)) && break
    sleep 1
done


# wait a couple second after the cgroup is created, just incase we might race kubernetes
sleep 5

# Ignore the output, incase
echo "Setting cgroup /sys/fs/cgroup/cpu/kubepods/ cpu.shares = 2"
echo 2 > /sys/fs/cgroup/cpu/kubepods/cpu.shares
