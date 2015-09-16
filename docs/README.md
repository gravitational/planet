# Planet Design

#### Intro

Planet is a small Golang executable. It is distributed as a part of an Orbit image, i.e. it 
expects to reside in `rootfs/usr/bin/planet`

When Planet starts, it:

* Uses [libcontainer](https://github.com/docker/libcontainer) to self-containerize 
* Launches [systemd](http://0pointer.de/blog/projects/systemd-docs.html)
* `systemd` instantiates Kubernetes services
* `systemd` also instantiates any other services we decide to run alongside Kubernetes

#### Planet Daemon

Once all of the above happen, Planet starts waiting for `systemd` to exit (see `Box` module).
Meanwhile, Planet also startsa web server which listens on a `/var/run/planet.sock` socket 
and is capable of serving requests like `stop`.

When another instance of Planet executes with `stop` command line argument, it connects to 
the socket to tell the running copy to stop.
