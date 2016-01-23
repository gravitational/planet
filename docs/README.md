# Planet Design

#### Intro

When Planet starts, it:

* Uses [libcontainer] to self-containerize 
* Launches [systemd] as the main process which manages the lifetime of all the other services - [Kubernetes]
and any other services we package together with it

#### Planet Daemon

Once started, planet will continue running and waiting for the main systemd process to end.
The container environment is encapsulated by the `[box]` package.
As part of its operation, planet will also start a web server to enable remote process control.
The server listens on the unix socket (in a `/var/run/planet.sock` by default) and is capable of
running commands inside the container on behalf of the client - this is how the commands `planet stop`
and `planet enter` are implemented.

#### Planet Agent

One additional recent planet service is the agent. Agent is responsible for maintaining the health status
of the cluster (obtainable with `planet status`) and implementing the master fail-over by dynamically promoting
nodes to master should the active master fail.


[//]: # (Footnots and references)

[systemd]: <http://www.freedesktop.org/wiki/Software/systemd/>
[libcontainer]: <https://github.com/opencontainers/runc/tree/master/libcontainer>
[Kubernetes]: <https://github.com/kubernetes/kubernetes>
[box]: <https://github.com/gravitational/planet/tree/master/lib/box>
