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
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gravitational/trace"
	"github.com/opencontainers/runc/libcontainer/user"
)

// This file implements edit functions for passwd/group files.

// LookupUID looks up a user by ID in the passwd database on the host.
func LookupUID(uid int) (*User, error) {
	u, err := lookupUser(func(u user.User) bool {
		return u.Uid == uid
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return (*User)(u), nil
}

// SysFile is a base interface of a passwd/group reader/writer.
type SysFile interface {
	Save(w io.Writer) error
}

// PasswdFile defines an interface to a passwd file.
type PasswdFile interface {
	SysFile

	// Upsert creates or updates the user in the passwd database
	Upsert(u User)
	// GetByID returns an existing user given its ID
	GetByID(id int) (u User, exists bool)
}

// GroupFile defines an interface to a group file.
type GroupFile interface {
	SysFile

	// Upsert creates or updates the group in the group database
	Upsert(g Group)
	// GetByID returns an existing group given its ID
	GetByID(id int) (g Group, exists bool)
}

type User user.User

// https://en.wikipedia.org/wiki/Passwd
func (u User) String() string {
	return fmt.Sprintf("%s:%s:%d:%d:%s:%s:%s",
		u.Name, u.Pass, u.Uid, u.Gid, u.Gecos, u.Home, u.Shell)
}

// passwdFile allows to read/write passwd files.
type passwdFile struct {
	users []user.User
}

var _ PasswdFile = (*passwdFile)(nil)

// NewPasswd creates a passwd file reader.
func NewPasswd(r io.Reader) (*passwdFile, error) {
	users, err := user.ParsePasswd(r)
	if err != nil {
		return nil, err
	}
	return &passwdFile{users: users}, nil
}

// Upsert adds a new or replaces an existing user.
func (r *passwdFile) Upsert(u User) {
	var found bool
	for i, usr := range r.users {
		if usr.Name == u.Name {
			r.users[i] = user.User(u)
			found = true
			break
		}
	}
	if !found {
		r.users = append(r.users, user.User(u))
	}
}

// GetID looks up existing user by ID.
// Upon success exists will also be set to true.
func (r *passwdFile) GetByID(id int) (u User, exists bool) {
	for _, user := range r.users {
		if user.Uid == id {
			return User(user), true
		}
	}
	return User{}, false
}

// Save stores the contents of this passwdFile into w.
func (r *passwdFile) Save(w io.Writer) (err error) {
	b := newBuffer(w)
	for _, user := range r.users {
		b.WriteLine(User(user).String())
	}
	if b.err == nil {
		err = b.Flush()
	}
	return b.err
}

// textLineBuffer simplifies the process of streaming lines of text into an io.Writer
type textLineBuffer struct {
	*bufio.Writer
	err error
}

func newBuffer(w io.Writer) *textLineBuffer {
	return &textLineBuffer{
		Writer: bufio.NewWriter(w),
	}
}

func (r *textLineBuffer) WriteLine(s string) {
	if r.err != nil {
		return
	}
	_, r.err = r.WriteString(s + "\n")
}

type Group user.Group

// http://www.cyberciti.biz/faq/understanding-etcgroup-file/
func (g Group) String() string {
	groups := strings.Join(g.List, ",")
	return fmt.Sprintf("%s:%s:%d:%s", g.Name, g.Pass, g.Gid, groups)
}

// groupFile allows to read/write group files.
type groupFile struct {
	groups []user.Group
}

var _ GroupFile = (*groupFile)(nil)

// NewGroup creates a group file reader.
func NewGroup(r io.Reader) (*groupFile, error) {
	groups, err := user.ParseGroup(r)
	if err != nil {
		return nil, err
	}
	return &groupFile{groups: groups}, nil
}

// Upsert adds a new or replaces an existing group.
func (r *groupFile) Upsert(g Group) {
	var found bool
	for i, group := range r.groups {
		if group.Name == g.Name {
			r.groups[i] = user.Group(g)
			found = true
			break
		}
	}
	if !found {
		r.groups = append(r.groups, user.Group(g))
	}
}

// GetID looks up existing group by ID.
// Upon success exists will also be set to true.
func (r *groupFile) GetByID(id int) (g Group, exists bool) {
	for _, group := range r.groups {
		if group.Gid == id {
			return Group(group), true
		}
	}
	return Group{}, false
}

// Save stores the contents of this groupFile into w.
func (r *groupFile) Save(w io.Writer) (err error) {
	b := newBuffer(w)
	for _, group := range r.groups {
		b.WriteLine(Group(group).String())
	}
	if b.err == nil {
		err = b.Flush()
	}
	return b.err
}

func lookupUser(filter func(u user.User) bool) (*user.User, error) {
	// Get operating system-specific passwd reader.
	passwd, err := getPasswdUbuntuCore()
	if err != nil && !trace.IsNotFound(err) {
		return nil, trace.Wrap(err)
	}
	if trace.IsNotFound(err) {
		passwd, err = user.GetPasswd()
		if err != nil {
			return nil, trace.ConvertSystemError(err)
		}
	}
	defer passwd.Close()

	// Get the users.
	users, err := user.ParsePasswdFilter(passwd, filter)
	if err != nil {
		return nil, trace.ConvertSystemError(err)
	}

	if len(users) == 0 {
		return nil, trace.NotFound("no matching entries in passwd file")
	}

	// Assume the first entry is the "correct" one.
	return &users[0], nil
}

func getPasswdUbuntuCore() (io.ReadCloser, error) {
	f, err := os.Open(ubuntuCorePasswdPath)
	if err != nil {
		return nil, trace.ConvertSystemError(err)
	}
	return f, nil
}

// Ubuntu-Core-specific path to the passwd formatted file.
const (
	ubuntuCorePasswdPath = "/var/lib/extrausers/passwd"
)
