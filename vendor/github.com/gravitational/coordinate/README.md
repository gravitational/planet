# Coordinate

Coordinate provides Etcd-based leader election with backoff and events:

```go
client, err := leader.NewClient(...)
if err != nil {
	return nil, trace.Wrap(err)
}

if err := client.AddVoter(conf.LeaderKey, "my id", conf.Term); err != nil {
	return nil, trace.Wrap(err)
}
// certain units must work only if the node is a master
client.AddWatchCallback(conf.LeaderKey, conf.Term/3, func(key, prevVal, newVal string) {
	if newVal == "my id" {
		log.Infof("i am leader now!")
	} else {
		log.Infof("%v just became a new leader", newVal)
	}
})
```
