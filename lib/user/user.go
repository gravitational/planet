package user

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/opencontainers/runc/libcontainer/user"
)

// This file implements edit functions for passwd/group files.

// SysFile is a base interface of a passwd/group reader/writer.
type SysFile interface {
	Save(w io.Writer) error
}

// PasswdFile defines an interface to a passwd file.
type PasswdFile interface {
	SysFile

	Upsert(u User)
	Get(name string) (u User, exists bool)
}

// GroupFile defines an interface to a group file.
type GroupFile interface {
	SysFile

	Upsert(g Group)
	Get(name string) (g Group, exists bool)
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

// Get looks up existing user and returns it.
// Upon success exists will also be set to true.
func (r *passwdFile) Get(name string) (u User, exists bool) {
	for _, user := range r.users {
		if user.Name == name {
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

// Get looks up existing group and returns it.
// Upon success exists will also be set to true.
func (r *groupFile) Get(name string) (g Group, exists bool) {
	for _, group := range r.groups {
		if group.Name == name {
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
