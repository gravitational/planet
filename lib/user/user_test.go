/*
Copyright 2018 Gravitational, Inc.

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

package user

import (
	"bytes"
	"strings"
	"testing"
)

const passwd = `root:x:0:0:root:/root:/bin/bash
daemon:x:1:1:daemon:/usr/sbin:/usr/sbin/nologin
bin:x:2:2:bin:/bin:/usr/sbin/nologin`

func TestReadsPasswdFile(t *testing.T) {
	rdr := strings.NewReader(passwd)
	r, err := NewPasswd(rdr)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.users) != 3 {
		t.Errorf("expected 3 users but got %d", len(r.users))
	}
}

func TestAddsUser(t *testing.T) {
	rw := bytes.NewBufferString(passwd)
	passwd, err := NewPasswd(rw)
	if err != nil {
		t.Fatal(err)
	}
	passwd.Upsert(newUser(1005, 1005))
	if len(passwd.users) != 4 {
		t.Error("expected to add a user")
	}
	rw.Reset()
	err = passwd.Save(rw)
	if err != nil {
		t.Fatal(err)
	}
	rdr := strings.NewReader(rw.String())
	passwd, err = NewPasswd(rdr)
	if err != nil {
		t.Fatal(err)
	}
	_, exists := passwd.GetByID(1005)
	if !exists {
		t.Fatal("expected to find a user")
	}
}

func TestReplacesUser(t *testing.T) {
	rw := bytes.NewBufferString(passwd)
	r, err := NewPasswd(rw)
	if err != nil {
		t.Fatal(err)
	}
	u := newUser(1005, 1005)
	r.Upsert(u)
	u2 := newUser(1006, 1006)
	r.Upsert(u2)
	if len(r.users) != 4 {
		t.Error("expected to replace a user")
	}
	rw.Reset()
	err = r.Save(rw)
	if err != nil {
		t.Fatal(err)
	}
	rdr := strings.NewReader(rw.String())
	r, err = NewPasswd(rdr)
	if err != nil {
		t.Fatal(err)
	}
	u3, exists := r.GetByID(1006)
	if !exists {
		t.Fatal("expected to find a user")
	}
	if u3.Uid != 1006 || u3.Gid != 1006 {
		t.Error("unexpected uid/gid for replaced user")
	}
}

func newUser(uid, gid int) User {
	return User{
		Name:  "planet-agent",
		Pass:  "x",
		Uid:   uid,
		Gid:   gid,
		Home:  "/home/planet-agent",
		Shell: "/bin/false",
	}
}
