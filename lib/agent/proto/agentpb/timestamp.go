package agentpb

import (
	"time"
)

func (ts Timestamp) ToTime() time.Time {
	return time.Unix(ts.Seconds, int64(ts.Nanoseconds)).UTC()
}

func TimeToProto(t time.Time) Timestamp {
	seconds := t.Unix()
	nanoseconds := int64(t.Sub(time.Unix(seconds, 0)))
	return Timestamp{
		Seconds:     seconds,
		Nanoseconds: int32(nanoseconds),
	}
}

func NewTimeToProto(t time.Time) *Timestamp {
	ts := new(Timestamp)
	*ts = TimeToProto(t)
	return ts
}

func (ts Timestamp) Equal(other Timestamp) bool {
	return ts.ToTime().Equal(other.ToTime())
}

// encoding.TextMarshaler
func (ts Timestamp) MarshalText() (text []byte, err error) {
	return ts.ToTime().MarshalText()
}

// encoding.TextUnmarshaler
func (ts *Timestamp) UnmarshalText(text []byte) error {
	t, err := time.Parse(time.RFC3339, string(text))
	if err != nil {
		return err
	}
	*ts = TimeToProto(t)
	return nil
}
