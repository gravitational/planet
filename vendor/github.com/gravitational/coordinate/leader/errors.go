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

	"go.etcd.io/etcd/client"
)

// IsNotFound determines if the specified error identifies a node not found event
func IsNotFound(err error) bool {
	e, ok := err.(client.Error)
	if !ok {
		return false
	}
	switch e.Code {
	case client.ErrorCodeKeyNotFound, client.ErrorCodeNotFile, client.ErrorCodeNotDir:
		return true
	default:
		return false
	}
}

// IsAlreadyExists determines if the specified error identifies a duplicate node event
func IsAlreadyExists(err error) bool {
	e, ok := err.(client.Error)
	if !ok {
		return false
	}
	return e.Code == client.ErrorCodeNodeExist
}

// IsWatchExpired determins if the specified error identifies an expired watch event
func IsWatchExpired(err error) bool {
	switch clientErr := err.(type) {
	case client.Error:
		return clientErr.Code == client.ErrorCodeEventIndexCleared
	}
	return false
}

// IsContextCanceled returns true if the provided error indicates canceled context.
func IsContextCanceled(err error) bool {
	if err == context.Canceled {
		return true
	}
	if clusterErr, ok := err.(*client.ClusterError); ok {
		if len(clusterErr.Errors) != 0 && clusterErr.Errors[0] == context.Canceled {
			return true
		}
	}
	return false
}
