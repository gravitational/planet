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
	osuser "os/user"
	"strconv"
	"strings"

	"github.com/gravitational/trace"
	"github.com/opencontainers/runc/libcontainer/user"
)

// LookupID returns user for the specified uid
func LookupID(uid string) (*User, error) {
	user, err := osuser.LookupId(uid)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return fromOSUser(*user)
}

// PasswdFile defines an interface to a passwd file.
type PasswdFile interface {
	io.WriterTo
	// Upsert creates or updates the user in the passwd database
	Upsert(u osuser.User)
}

// GroupFile defines an interface to a group file.
type GroupFile interface {
	io.WriterTo
	// Upsert creates or updates the group in the group database
	Upsert(g osuser.Group)
}

// User defines a user data type
type User user.User

// Group returns group this user belongs to
func (u User) Group() (*Group, error) {
	group, err := osuser.LookupGroupId(strconv.Itoa(u.Gid))
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return fromOSGroup(*group)
}

// String returns the user info in /etc/passwd format
func (u User) String() string {
	return fmt.Sprintf("%v:%v:%v:%v:%v:%v:%v",
		u.Name,
		u.Pass,
		u.Uid,
		u.Gid,
		u.Gecos,
		u.Home,
		u.Shell)
}

// fromOSUser converts stdlib user to User
func fromOSUser(u osuser.User) (*User, error) {
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &User{
		Name:  u.Username,
		Pass:  passPlaceholder,
		Uid:   uid,
		Gid:   gid,
		Gecos: u.Name,
		Home:  u.HomeDir,
	}, nil
}

// passwdFile allows to read/write passwd files.
type passwdFile struct {
	users []User
}

// NewPasswd creates a passwd file reader.
func NewPasswd(r io.Reader) (*passwdFile, error) {
	parsed, err := user.ParsePasswd(r)
	if err != nil {
		return nil, err
	}
	var users []User
	for _, u := range parsed {
		users = append(users, User(u))
	}
	return &passwdFile{users: users}, nil
}

// NewPasswdFromFile creates a passwd file reader from provided path
func NewPasswdFromFile(path string) (*passwdFile, error) {
	reader, err := os.Open(path)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	defer reader.Close()
	return NewPasswd(reader)
}

// Upsert adds a new or replaces an existing user.
func (r *passwdFile) Upsert(u User) {
	for i, usr := range r.users {
		if usr.Name == u.Name {
			r.users[i] = u
			return
		}
	}
	r.users = append(r.users, u)
}

// Save stores the contents of this passwdFile into w.
func (r *passwdFile) WriteTo(w io.Writer) (n int64, err error) {
	b := newBuffer(w)
	for _, user := range r.users {
		n += int64(b.WriteLine(user.String()))
	}
	if b.err == nil {
		err = b.Flush()
	}
	return n, trace.Wrap(b.err)
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

func (r *textLineBuffer) WriteLine(s string) (n int) {
	if r.err != nil {
		return 0
	}
	n, r.err = r.WriteString(s + "\n")
	return n
}

// Group defines a group data type
type Group user.Group

// String returns the group info in /etc/group format
func (g Group) String() string {
	return fmt.Sprintf("%s:%s:%d:%s",
		g.Name,
		g.Pass,
		g.Gid,
		strings.Join(g.List, ","))
}

// fromOSGroup converts stdlib group to Group
func fromOSGroup(g osuser.Group) (*Group, error) {
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &Group{
		Name: g.Name,
		Pass: passPlaceholder,
		Gid:  gid,
	}, nil
}

// groupFile allows to read/write group files.
type groupFile struct {
	groups []Group
}

// NewGroup creates a group file reader.
func NewGroup(r io.Reader) (*groupFile, error) {
	parsed, err := user.ParseGroup(r)
	if err != nil {
		return nil, err
	}
	var groups []Group
	for _, g := range parsed {
		groups = append(groups, Group(g))
	}
	return &groupFile{groups: groups}, nil
}

// NewGroupFromFile creates a group file reader from provided path
func NewGroupFromFile(path string) (*groupFile, error) {
	reader, err := os.Open(path)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	defer reader.Close()
	return NewGroup(reader)
}

// Upsert adds a new or replaces an existing group.
func (r *groupFile) Upsert(g Group) {
	for i, group := range r.groups {
		if group.Name == g.Name {
			r.groups[i] = g
			return
		}
	}
	r.groups = append(r.groups, g)
}

// Save stores the contents of this groupFile into w.
func (r *groupFile) WriteTo(w io.Writer) (n int64, err error) {
	b := newBuffer(w)
	for _, group := range r.groups {
		n += int64(b.WriteLine(group.String()))
	}
	if b.err == nil {
		err = b.Flush()
	}
	return n, trace.Wrap(b.err)
}

// passPlaceholder is the password field placeholder user in passwd/group files
const passPlaceholder = "x"
