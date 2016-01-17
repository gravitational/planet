#! /bin/bash
protoc_home=~/opt/google/protobuf/bin
$protoc_home/protoc --go_out=plugins=grpc,import_prefix=github.com/gravitational/planet/Godeps/_workspace/src/:. agentpb/agent.proto
