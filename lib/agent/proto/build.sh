#! /bin/bash
~/opt/google/protobuf/bin/protoc --go_out=plugins=grpc:. agentpb/agent.proto
