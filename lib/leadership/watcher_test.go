package leadership

import (
	"context"

	v3 "go.etcd.io/etcd/clientv3"
	. "gopkg.in/check.v1"
)

func (s *S) TestCanStopWatcher(c *C) {
	w, err := NewWatcher(WatcherConfig{
		Key:    "testkey",
		Client: s.client,
	})
	c.Assert(err, IsNil)
	w.Start()
	w.Stop()
}

func (s *S) TestWatcherNotifiesOfValueUpdates(c *C) {
	w, err := NewWatcher(WatcherConfig{
		Key:    "testkey",
		Client: s.client,
	})
	c.Assert(err, IsNil)
	w.Start()
	defer w.Stop()
	respChan := make(chan v3.Event, 2)
	go func() {
		for resp := range w.RespChan() {
			respChan <- resp
		}
	}()
	_, err = s.client.Put(context.TODO(), "testkey", "value1")
	c.Assert(err, IsNil)
	obtained := <-respChan
	c.Assert(string(obtained.Kv.Value), Equals, "value1")
	_, err = s.client.Put(context.TODO(), "testkey", "value2")
	c.Assert(err, IsNil)
	obtained = <-respChan
	c.Assert(string(obtained.Kv.Value), Equals, "value2")
}
