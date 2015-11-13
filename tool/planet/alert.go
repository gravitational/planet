package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
)

const alertsFile = "/etc/monit/alerts"

type alertMessage struct {
	Timestamp time.Time `json:"timestamp"`
	Module    string    `json:"message"`
	Reason    string    `json:"reason"`
}

// alert adds an alert message for `module` to alerts file.
func alert(module, reason string) error {
	var message []byte
	var f *os.File
	var err error
	item := &alertMessage{
		Timestamp: time.Now().UTC(),
		Module:    module,
		Reason:    reason,
	}

	f, err = os.OpenFile(alertsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return trace.Wrap(err, fmt.Sprintf("cannot open alerts file `%s`", alertsFile))
	}
	defer f.Close()

	message, err = json.Marshal(item)
	if err != nil {
		return trace.Wrap(err)
	}
	message = append(message, '\n')

	log.Infof(item.String())
	_, err = f.Write(message)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

func (r *alertMessage) String() string {
	return fmt.Sprintf("alert(module=%s, reason=%s, when=%s)", r.Module, r.Reason, r.Timestamp)
}
