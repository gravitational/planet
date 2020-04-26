/*
Copyright 2020 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package leader

import (
	"context"

	"github.com/coreos/etcd/client"
	"github.com/gravitational/trace"
)

// IsNotFound determines if the specified error identifies a node not found event
func IsNotFound(err error) bool {
	clientErr, ok := trace.Unwrap(err).(client.Error)
	if !ok {
		return false
	}
	switch clientErr.Code {
	case client.ErrorCodeKeyNotFound, client.ErrorCodeNotFile, client.ErrorCodeNotDir:
		return true
	default:
		return false
	}
}

// IsAlreadyExists determines if the specified error identifies a duplicate node event
func IsAlreadyExists(err error) bool {
	if err, ok := trace.Unwrap(err).(client.Error); ok {
		return err.Code == client.ErrorCodeNodeExist
	}
	return false
}

// IsWatchExpired determins if the specified error identifies an expired watch event
func IsWatchExpired(err error) bool {
	if err, ok := trace.Unwrap(err).(client.Error); ok {
		return err.Code == client.ErrorCodeEventIndexCleared
	}
	return false
}

// IsContextError returns true if the provided error indicates a canceled or expired context.
func IsContextError(err error) bool {
	if err == nil {
		return false
	}
	err = trace.Unwrap(err)
	if err == context.Canceled || err == context.DeadlineExceeded {
		return true
	}
	if err, ok := err.(*client.ClusterError); ok {
		return len(err.Errors) != 0 && (err.Errors[0] == context.Canceled ||
			err.Errors[0] == context.DeadlineExceeded)
	}
	return false
}
