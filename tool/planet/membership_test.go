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

package main

import (
	"net"

	"github.com/gravitational/planet/lib/test"

	serf "github.com/hashicorp/serf/client"
	"gopkg.in/check.v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type MembershipSuite struct{}

var _ = check.Suite(&MembershipSuite{})

func (r *MembershipSuite) TestReconcile(c *check.C) {
	var tests = []struct {
		comment   string
		k8sNodes  []v1.Node
		serfNodes []serf.Member
		expected  []serf.Member
	}{
		{
			comment: "Member list should not change.",
			k8sNodes: []v1.Node{
				r.newK8sNode("172.28.128.101"),
				r.newK8sNode("172.28.128.102"),
			},
			serfNodes: []serf.Member{
				r.newSerfNode("172.28.128.101"),
				r.newSerfNode("172.28.128.102"),
			},
			expected: []serf.Member{
				r.newSerfNode("172.28.128.101"),
				r.newSerfNode("172.28.128.102"),
			},
		},
		{
			comment: "Join a single missing node to serf cluster.",
			k8sNodes: []v1.Node{
				r.newK8sNode("172.28.128.101"),
				r.newK8sNode("172.28.128.102"),
			},
			serfNodes: []serf.Member{
				r.newSerfNode("172.28.128.101"),
			},
			expected: []serf.Member{
				r.newSerfNode("172.28.128.101"),
				r.newSerfNode("172.28.128.102"),
			},
		},
		{
			comment: "Join all missing nodes to the serf cluster.",
			k8sNodes: []v1.Node{
				r.newK8sNode("172.28.128.101"),
				r.newK8sNode("172.28.128.102"),
			},
			expected: []serf.Member{
				r.newSerfNode("172.28.128.101"),
				r.newSerfNode("172.28.128.102"),
			},
		},
	}

	for _, tc := range tests {
		comment := check.Commentf(tc.comment)
		k8sClient := fake.NewSimpleClientset(&v1.NodeList{Items: tc.k8sNodes})
		serfClient := &mockSerfClient{members: tc.serfNodes}

		c.Assert(reconcileSerf(k8sClient, serfClient), check.IsNil, comment)

		members, err := serfClient.Members()
		c.Assert(err, check.IsNil, comment)
		c.Assert(members, test.DeepEquals, tc.expected, comment)
	}
}

// newK8sNode constructs a new kubernetes node with advertised-ip mapped to
// the provided addr.
func (r *MembershipSuite) newK8sNode(addr string) v1.Node {
	return v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   addr, // Node names must be unique but is not needed for tests.
			Labels: map[string]string{advertiseIPKey: addr},
		},
	}
}

// newSerfNode constructs a new serf node.
func (r *MembershipSuite) newSerfNode(addr string) serf.Member {
	return serf.Member{Addr: net.ParseIP(addr)}
}

type mockSerfClient struct {
	members []serf.Member
}

// Members returns the lsit of members.
func (r *mockSerfClient) Members() (members []serf.Member, err error) {
	return r.members, nil
}

// Join joins the nodes the the cluster.
func (r *mockSerfClient) Join(peers []string, replay bool) (joined int, err error) {
	existing := make(map[string]struct{})
	for _, member := range r.members {
		existing[member.Addr.String()] = struct{}{}
	}
	for _, peer := range peers {
		if _, exists := existing[peer]; exists {
			continue
		}
		r.members = append(r.members, serf.Member{Addr: net.ParseIP(peer)})
	}
	return joined, nil
}

const (
	node1 = "node-1"
	node2 = "node-2"
)
