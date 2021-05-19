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

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

/*
This script compresses the contents of the specified directory
using the following rules:

fakeroot -- sh -c ' \
  chown -R $(PLANET_UID):$(PLANET_GID) . ; \
  chown -R root:root rootfs/sbin/mount.* ; \
  chown -R root:$(PLANET_GID) rootfs/etc/ ; \
  chmod -R g+rw rootfs/etc ; \
  tar -czf $(TARBALL) orbit.manifest.json rootfs'
*/

func main() {
	if len(os.Args) < 3 {
		log.Fatalln("Use: create-tarball <rootfs-dir> <output-tarball>")
	}

	rootDir, outputTarball := os.Args[1], os.Args[2]
	if err := run(rootDir, outputTarball); err != nil {
		log.Fatalln(trace.DebugReport(err))
	}
}

func run(rootDir, outputTarball string) (err error) {
	f, err := os.Create(outputTarball)
	if err != nil {
		return trace.ConvertSystemError(err)
	}
	defer func() {
		if errClose := f.Close(); errClose != nil {
			err = errClose
		}
	}()
	zip := gzip.NewWriter(f)
	archive := tar.NewWriter(zip)
	defer archive.Close()
	defer zip.Close()

	if err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return trace.Wrap(err)
		}
		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			return trace.Wrap(err)
		}
		if relPath == "." {
			// Do not add the current directory entry
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return trace.Wrap(err)
		}
		if relPath == "orbit.manifest.json" {
			return storeManifest(path, relPath, fi, archive)
		}
		var link string
		if fi.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(path)
			if err != nil {
				return trace.Wrap(trace.ConvertSystemError(err))
			}
			log.Debug("Symlink ", link)
		}
		hdr, err := fileInfoHeader(relPath, fi, link)
		if err != nil {
			return trace.Wrap(err)
		}
		for _, p := range patterns {
			if !p.matches(relPath) {
				continue
			}
			if p.exclude {
				log.Debug("Excluding ", relPath)
				return nil
			}
			hdr = p.updateHeader(*hdr)
			log.Debug("Found match ", path, " for ", p)
			break
		}
		log.Debug("Adding ", relPath)
		if err := archive.WriteHeader(hdr); err != nil {
			return trace.Wrap(err)
		}
		if d.IsDir() {
			// Continue to next entry
			return nil
		}
		if hdr.Typeflag == tar.TypeReg && hdr.Size > 0 {
			f, err := os.Open(path)
			if err != nil {
				return trace.ConvertSystemError(err)
			}
			if _, err := io.Copy(archive, f); err != nil {
				f.Close()
				return trace.Wrap(trace.ConvertSystemError(err), "failed to copy contents of %s", path)
			}
			f.Close()
		}
		return nil
	}); err != nil {
		return trace.Wrap(trace.ConvertSystemError(err))
	}
	return nil
}

func fileInfoHeader(relPath string, fi fs.FileInfo, link string) (*tar.Header, error) {
	hdr, err := tar.FileInfoHeader(fi, link)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	hdr.Uid = *planetUID
	hdr.Gid = *planetGID
	// Reset the username/group as these are explicitly set to uid/gid above,
	// otherwise the tar writer will prioritize the symbolic names
	hdr.Uname = ""
	hdr.Gname = ""
	hdr.Format = tar.FormatPAX
	hdr.ModTime = hdr.ModTime.Truncate(time.Second)
	hdr.AccessTime = time.Time{}
	hdr.ChangeTime = time.Time{}
	hdr.Mode = fillGo18FileTypeBits(int64(os.FileMode(hdr.Mode)), fi)
	hdr.Name = relPath
	return hdr, nil
}

// fillGo18FileTypeBits fills type bits which have been removed on Go 1.9 archive/tar
// https://github.com/golang/go/commit/66b5a2f
func fillGo18FileTypeBits(mode int64, fi os.FileInfo) int64 {
	fm := fi.Mode()
	switch {
	case fm.IsRegular():
		mode |= modeISREG
	case fi.IsDir():
		mode |= modeISDIR
	case fm&os.ModeSymlink != 0:
		mode |= modeISLNK
	case fm&os.ModeDevice != 0:
		if fm&os.ModeCharDevice != 0 {
			mode |= modeISCHR
		} else {
			mode |= modeISBLK
		}
	case fm&os.ModeNamedPipe != 0:
		mode |= modeISFIFO
	case fm&os.ModeSocket != 0:
		mode |= modeISSOCK
	}
	return mode
}

var patterns = []pattern{
	{
		uid:  planetUID,
		gid:  planetGID,
		name: "orbit.manifest.json",
	},
	{
		uname:  rootUname,
		gid:    planetGID,
		prefix: "rootfs/etc",
		// g+rw
		perms: 060,
	},
	{
		uname:  rootUname,
		gname:  rootGname,
		prefix: "rootfs/sbin/mount.",
	},
	{
		prefix:  "rootfs/tmp/",
		exclude: true,
	},
	// Shrink rules
	/*
		rm -rf /usr/share/man
		rm -rf /usr/share/doc
		rm -rf /var/lib/apt
		rm -rf /var/log/*
		rm -rf /var/cache
		rm -rf /usr/share/locale
		rm -rf /lib/systemd/system/sysinit.target.wants/proc-sys-fs-binfmt_misc.automount
		rm -rf /lib/modules-load.d/open-iscsi.conf
	*/
	{
		prefix:  "rootfs/usr/share/man",
		exclude: true,
	},
	{
		prefix:  "rootfs/usr/share/doc",
		exclude: true,
	},
	{
		prefix:  "rootfs/var/lib/apt",
		exclude: true,
	},
	{
		// Include the trailing slash to only match contents
		prefix:  "rootfs/var/log/",
		exclude: true,
	},
	{
		prefix:  "rootfs/var/cache",
		exclude: true,
	},
	{
		prefix:  "rootfs/usr/share/locale",
		exclude: true,
	},
	{
		name:    "rootfs/lib/systemd/system/sysinit.target.wants/proc-sys-fs-binfmt_misc.automount",
		exclude: true,
	},
	{
		name:    "rootfs/lib/modules-load.d/open-iscsi.conf",
		exclude: true,
	},
}

func (r pattern) String() string {
	if r.name != "" {
		return r.name
	}
	return r.prefix
}

func (r pattern) matches(path string) bool {
	if r.name != "" {
		return path == r.name
	}
	if r.prefix == "" {
		panic("empty prefix")
	}
	return strings.HasPrefix(path, r.prefix)

}

type pattern struct {
	uid, gid     *int
	uname, gname string
	// entry perms
	perms os.FileMode
	// prefix optionally specifies the directory prefix relative
	// to the root directory.
	prefix string
	// name optionally specifies the filename relative to the root directory
	name string
	// exclude optionally specifies whether the match should be explicitly skipped
	exclude bool
}

func (r pattern) updateHeader(hdr tar.Header) *tar.Header {
	if r.uid != nil {
		hdr.Uid = *r.uid
		hdr.Uname = ""
	}
	if r.gid != nil {
		hdr.Gid = *r.gid
		hdr.Gname = ""
	}
	if r.uname != "" {
		hdr.Uid = 0 // explicitly reset
		hdr.Uname = r.uname
	}
	if r.gname != "" {
		hdr.Gid = 0 // explicitly reset
		hdr.Gname = r.gname
	}
	if r.perms != 0 {
		hdr.Mode |= int64(r.perms & fs.ModePerm)
	}
	return &hdr
}

const (
	rootUname = "root"
	rootGname = "root"

	modeISDIR  = 040000  // Directory
	modeISFIFO = 010000  // FIFO
	modeISREG  = 0100000 // Regular file
	modeISLNK  = 0120000 // Symbolic link
	modeISBLK  = 060000  // Block special file
	modeISCHR  = 020000  // Character special file
	modeISSOCK = 0140000 // Socket
)

var (
	rootUID   = intPtr(0)
	planetUID = intPtr(980665)
	planetGID = intPtr(980665)
)

func storeManifest(path, relPath string, fi fs.FileInfo, archive *tar.Writer) error {
	f, err := os.Open(path)
	if err != nil {
		return trace.Wrap(trace.ConvertSystemError(err))
	}
	defer f.Close()
	var m manifest
	dec := json.NewDecoder(f)
	if err := dec.Decode(&m); err != nil {
		return trace.Wrap(err, "failed to read manifest")
	}
	// envmap maps REPLACE_xxx envars to their value
	envmap := make(map[string]string)
	for _, env := range os.Environ() {
		split := strings.Split(env, "=")
		k, v := split[0], split[1]
		if strings.HasPrefix(k, "REPLACE_") {
			envmap[k] = v
		}
	}
	for i, lbl := range m.Labels {
		if version, ok := envmap[lbl.Value]; ok {
			m.Labels[i].Value = version
		}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(m); err != nil {
		return trace.Wrap(err, "failed to encode manifest")
	}
	hdr, err := fileInfoHeader(relPath, fi, "")
	if err != nil {
		return trace.Wrap(err)
	}
	hdr.Size = int64(buf.Len())
	if err := archive.WriteHeader(hdr); err != nil {
		return trace.Wrap(err)
	}
	_, err = archive.Write(buf.Bytes())
	return trace.Wrap(err)
}

// manifest captures a part of the planet manifest
type manifest struct {
	Version string `json:"version"`
	Labels  []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"labels"`
	Commands json.RawMessage `json:"commands"`
	Service  json.RawMessage `json:"service"`
	Config   json.RawMessage `json:"config"`
}

func intPtr(v int) *int {
	return &v
}
