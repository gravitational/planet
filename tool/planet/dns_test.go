/*
Copyright 2022 Gravitational, Inc.

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

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCreateLabelSelector(t *testing.T) {
	tests := []struct {
		labels   []string
		expected metav1.LabelSelector
		panics   bool
	}{
		{
			labels: []string{"k8s-app", "kube-dns"},
			expected: metav1.LabelSelector{
				MatchLabels: map[string]string{"k8s-app": "kube-dns"},
			},
			panics: false,
		},
		{
			labels: []string{"k8s-app", "kube-dns-worker"},
			expected: metav1.LabelSelector{
				MatchLabels: map[string]string{"k8s-app": "kube-dns-worker"},
			},
			panics: false,
		},
		{
			labels: []string{"k8s-app"},
			panics: true,
		},
	}

	for _, tt := range tests {
		if tt.panics {
			assert.Panics(t, func() { mustLabelSelector(tt.labels...) }, "the code did not panic")
			continue
		}

		expectedSelector, err := metav1.LabelSelectorAsSelector(&tt.expected)
		assert.NoError(t, err)

		actualSelector := mustLabelSelector(tt.labels...)
		assert.Equal(t, expectedSelector, actualSelector)
	}
}
