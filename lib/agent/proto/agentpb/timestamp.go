package agentpb

import (
	"time"
)

// ToTime converts this timestamp to time.Time value.
func (ts Timestamp) ToTime() time.Time {
	return time.Unix(ts.Seconds, int64(ts.Nanoseconds)).UTC()
}

// TimeToTime creates a new instance of Timestamp from the given time.Time value.
func TimeToProto(t time.Time) Timestamp {
	seconds := t.Unix()
	nanoseconds := int64(t.Sub(time.Unix(seconds, 0)))
	return Timestamp{
		Seconds:     seconds,
		Nanoseconds: int32(nanoseconds),
	}
}

// NewTimeToProto is like TimeToProto but returns a pointer result instead.
func NewTimeToProto(t time.Time) *Timestamp {
	ts := new(Timestamp)
	*ts = TimeToProto(t)
	return ts
}

// NewTimestamp returns a timestamp set to current time.
func NewTimestamp() *Timestamp {
	ts := new(Timestamp)
	*ts = TimeToProto(time.Now())
	return ts
}

// Equal compares this timestamp with other to determine if they're equal.
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

// Clone returns a copy of this timestamp.
func (ts *Timestamp) Clone() (result *Timestamp) {
	result = new(Timestamp)
	*result = *ts
	return result
}
