#!/bin/bash

ln -sf /run/systemd/resolve/resolv.conf /etc/resolv.conf
echo "127.0.0.1 localhost" > /etc/hosts
