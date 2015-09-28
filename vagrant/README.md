### Vagrant

If you want to build&run `Planet` inside a VM, this Vagrant environment is for you.
It creates a VM which is:
    * based on `Debian Jessie`
    * pre-configured with Docker, which is needed to build Planet
    * installs Golang + dev packages (`vi`, `screen`, etc)
    * configures `.bashrc` to not suck
    * has your `~/.ssh` and `~/.gitconfig`

To start, have Vagrant v1.4+ and run:

```
vagrant up
```

Have fun.
