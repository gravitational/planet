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
	_, err = passwd.WriteTo(rw)
	if err != nil {
		t.Fatal(err)
	}
	rdr := strings.NewReader(rw.String())
	passwd, err = NewPasswd(rdr)
	if err != nil {
		t.Fatal(err)
	}
	var exists bool
	for _, u := range passwd.users {
		if u.Uid == 1005 {
			exists = true
		}
	}
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
	_, err = r.WriteTo(rw)
	if err != nil {
		t.Fatal(err)
	}
	rdr := strings.NewReader(rw.String())
	r, err = NewPasswd(rdr)
	if err != nil {
		t.Fatal(err)
	}
	var exists bool
	for _, u := range r.users {
		if u.Uid == 1006 {
			exists = true
		}
	}
	if !exists {
		t.Fatal("expected to find a user")
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
