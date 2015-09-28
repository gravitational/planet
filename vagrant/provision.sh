#!/bin/bash
BASHRC="/home/vagrant/.bashrc"
export GOROOT=/opt/go
export GOPATH=~/go
export PATH=$PATH:$GOROOT/bin:$GOPATH/bin

# upgrade packages only once in a while:
sudo apt-get -y update 
sudo apt-get -y upgrade

# install some packages we always need:
sudo apt-get -y install aptitude vim htop curl git-core tree

# colorize the prompt
if ! grep -q provisioned ~/.bashrc ; then 
    echo "export PS1='\[\033[34;1m\]v-\h\[\033[0;33m\] \w\[\033[00m\]: '" >> ~/.bashrc
    echo "export PATH=\"\$PATH:/usr/lib/git-core\"" >> ~/.bashrc
    echo "alias g=\"cd ~/go/src/github.com/gravitational\"" >> ~/.bashrc
    echo "alias p=\"cd ~/go/src/github.com/gravitational/planet\"" >> ~/.bashrc
    echo "alias ll=\"ls -lh\"" >> ~/.bashrc

    echo "# provisioned" >> ~/.bashrc
fi

# download go and place it into /opt
if [ ! -f /opt/go/bin/go ]; then
    echo "Downloading Go..."
    cd /tmp && curl --silent https://storage.googleapis.com/golang/go1.4.2.linux-amd64.tar.gz | tar -xz
    sudo mv /tmp/go $GOROOT
    sudo chown vagrant:vagrant $GOROOT
fi

# configure GOROOT and PATH to include the install directory for Golang
if ! grep -q GOPATH $BASHRC ; then 
    mkdir -p $GOPATH/bin $GOPATH/src 
    echo -e "\n# Go vars" >> $BASHRC
    echo "export GOROOT=$GOROOT" >> $BASHRC
    echo "export GOPATH=$GOPATH" >> $BASHRC
    echo "export PATH=\$PATH:\$GOROOT/bin:\$GOPATH/bin" >> $BASHRC
fi
