/*
Copyright 2021 Gravitational, Inc.

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

package utils

import (
	"github.com/gravitational/trace"

	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
)

// GetRegion returns the region the instance is running in.
func GetRegion() (string, error) {
	session, err := session.NewSession()
	if err != nil {
		return "", trace.Wrap(err)
	}
	metadata := ec2metadata.New(session)
	region, err := metadata.Region()
	if err != nil {
		return "", trace.Wrap(err)
	}
	return region, nil
}
