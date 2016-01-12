package proto

import (
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
	"time"
)

func ProtoToTime(ts *pb.Timestamp) time.Time {
	return time.Unix(ts.Seconds, int64(ts.Nanoseconds)).UTC()
}

func TimeToProto(t time.Time) *pb.Timestamp {
	seconds := t.Unix()
	nanoseconds := int64(t.Sub(time.Unix(seconds, 0)))
	return &pb.Timestamp{
		Seconds:     seconds,
		Nanoseconds: int32(nanoseconds),
	}
}
