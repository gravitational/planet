# End to end testing an existing Kubernetes cluster 

[Kubernetes] comes with a suite of end-to-end (e2e for short) tests that can be leveraged to quickly determine if the cluster we built is actually functioning - i.e. that the master and the nodes are all up and can communicate with each other.

Kubernetes e2e tests are written using a BDD framework called [ginkgo] using a DSL reminiscent of [jasmine] - a popular javascript BDD framework. Ginkgo tightly integrates with Go's native testing package and allows one to run the tests using `go test`, but for more advanced usage (for instance, being able to run tests in parallel), the test runner (ginkgo CLI) is required.
All Kubernetes e2e tests are precompiled inside of the test executable:

```sh
$ ls build/dev/test

-rwxr-xr-x 1 35899400 Okt 28 03:20 e2e.test
-rwxr-xr-x 1  9523192 Okt 28 03:20 ginkgo
```

# Tests

The tests are subdivided into groups. A feature of [ginkgo] allows one to focus on a specific group when executing tests (specs, in ginkgo terminology).
Following are some of the more interesting groups available as of this writing:

 - Deployment
  - creating new pods
  - deleting old and creating new pods
  - correct order up/down scaling
 - DNS (*)
  - provision of DNS to cluster / services
 - Docker Containers
 - Etcd failure
 - Events
  - ensuring events are generated
 - Job (*)
 - Kubectl client
   - Simple pod
   - Proxy server
   - extensive kubectl commands test
 - KubeProxy
 - Load capacity
  - random scaling of RC
 - Monitoring
  - monitoring nodes / pods on influxdb w/ heapster
 - Resource usage of system containers
 - Namespaces
 - Networking (*)
  - providing internet for containers
  - intra-pod communication
 - Pod Disks
  - scheduling pods with RW/R PDs or PDs shared between pods, etc.
 - Pods (*)
  - pod liveness tests
  - pods getting host IPs
  - being able to schedule pods with memory/cpu limits
  - submitting / removing pods
  - updating pods
  - support for remote command execution
  - being able to retrieve logs
 - PrivilegedPod (*)
  - retrieving ssh-able hosts
  - executing privileged commands on a (non-)privileged pod
 - Port forwarding (*)
 - Proxy
 - ReplicationController (*)
  - serving public/private images to each replica
 - Reboot (*)
  - nodes surviving restarts
  - nodes surviving kernel panic
  - nodes surviving network shutdown/restart
  - surviving after dropping inbound/outbound packets
 - Restart
  - restart of all nodes - ensuring nodes / pods recover
 - Nodes (*)
   - Resize
    - deleting/adding nodes
   - Network
    - rescheduling pods from nodes that cannot be contacted
    - hosting new pods on a node that joins a cluster
 - Secrets
 - Services (*)
  - serving endpoints from pods
  - up/downing services
  - working after restart of {kube proxy,api server}
 - SSH

Tests marked with an asterisk are the likely candidates for planet tests.


# Testing planet

Planet supports a `test` command that will ultimately perform an internal self-health check.
For now, it simply proxies commands to [ginkgo]. Here's how a specific test group can be run:

 - either by invoking `make test SPEC=<regexp>` that runs the test inside a docker container
  - for instance, `make test SPEC=Pods`
 - or by using `planet test` directly:
  -
  ```sh
  planet test
    --tool-dir=path/to/dir/with/runner
    --kube-master=127.0.0.1:8080 \
    --kube-config=path/to/kubeconfig \
    --kube-repo=path/to/kube/repo -- -focus=Pods
  ```

[//]: # (Footnots and references)

[Kubernetes]: <https://github.com/kubernetes/kubernetes>
[ginkgo]: <https://github.com/onsi/ginkgo>
[jasmine]: <http://jasmine.github.io>
[Go]: <https://golang.org>
	
