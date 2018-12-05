package utils

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WriteHosts formats entries in hosts file format to writer
func WriteHosts(writer io.Writer, entries []HostEntry) error {
	for _, entry := range entries {
		line := fmt.Sprintf("%v %v", entry.IP, entry.Hostnames)
		if _, err := io.WriteString(writer, line+"\n"); err != nil {
			return trace.ConvertSystemError(err)
		}
	}
	return nil
}

// HostEntry maps a list of hostnames to an IP
type HostEntry struct {
	// Hostnames is a list of space separated hostnames
	Hostnames string
	// IP is the IP the hostnames should resolve to
	IP string
}

// WriteDropIn creates the file specified with dropInPath in directory specified with dropInDir
// with given contents
func WriteDropIn(dropInDir, dropInFile string, contents []byte) error {
	err := os.MkdirAll(dropInDir, constants.SharedDirMask)
	if err != nil {
		return trace.ConvertSystemError(err)
	}

	dropInPath := filepath.Join(dropInDir, dropInFile)
	err = ioutil.WriteFile(dropInPath, contents, constants.SharedReadMask)
	if err != nil {
		return trace.ConvertSystemError(err)
	}

	return nil
}

// DropInDir returns the name of the directory for the specified unit
func DropInDir(unit string) string {
	return fmt.Sprintf("%v.d", unit)
}

// SafeWriteFile is similar to ioutil.WriteFile, but operates by writing to a temporary file first
// and then relinking the file into the filename which should be an atomic operation. This should be
// safer, that if the destination file is being replaced, it won't be left in a partial written state.
func SafeWriteFile(filename string, w io.WriterTo, perm os.FileMode) error {
	dir := filepath.Dir(filename)

	tmpFile, err := ioutil.TempFile(dir, "safewrite")
	if err != nil {
		return trace.ConvertSystemError(err)
	}
	defer os.Remove(tmpFile.Name())

	_, err = w.WriteTo(tmpFile)
	if err != nil {
		return trace.ConvertSystemError(err)
	}

	err = os.Chmod(tmpFile.Name(), perm)
	if err != nil {
		return trace.ConvertSystemError(err)
	}

	err = os.Rename(tmpFile.Name(), filename)
	if err != nil {
		return trace.ConvertSystemError(err)
	}

	return nil
}

// CopyFile copies contents of src to dst atomically
// using SharedReadWriteMask as permissions.
func CopyFile(dst, src string) error {
	return CopyFileWithPerms(dst, src, constants.SharedReadWriteMask)
}

// CopyReader copies contents of src to dst atomically
// using SharedReadWriteMask as permissions.
func CopyReader(dst string, src io.Reader) error {
	return CopyReaderWithPerms(dst, src, constants.SharedReadWriteMask)
}

// CopyFileWithPerms copies the contents from src to dst atomically.
// Uses CopyReaderWithPerms for its implementation - see function documentation
// for details of operation
func CopyFileWithPerms(dst, src string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return trace.ConvertSystemError(err)
	}
	defer in.Close()
	return CopyReaderWithPerms(dst, in, perm)
}

// CopyReaderWithPerms copies the contents from src to dst atomically.
// If dst does not exist, CopyReaderWithPerms creates it with permissions perm.
// If the copy fails, CopyReaderWithPerms aborts and dst is preserved.
// Adopted with modifications from https://go-review.googlesource.com/#/c/1591/9/src/io/ioutil/ioutil.go
func CopyReaderWithPerms(dst string, src io.Reader, perm os.FileMode) error {
	tmp, err := ioutil.TempFile(filepath.Dir(dst), "")
	if err != nil {
		return trace.ConvertSystemError(err)
	}

	cleanup := func() {
		err := os.Remove(tmp.Name())
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
				"file":  tmp.Name(),
			}).Warn("Failed to remove.")
		}
	}

	_, err = io.Copy(tmp, src)
	if err != nil {
		tmp.Close()
		cleanup()
		return trace.ConvertSystemError(err)
	}
	if err = tmp.Close(); err != nil {
		cleanup()
		return trace.ConvertSystemError(err)
	}
	if err = os.Chmod(tmp.Name(), perm); err != nil {
		cleanup()
		return trace.ConvertSystemError(err)
	}
	err = os.Rename(tmp.Name(), dst)
	if err != nil {
		cleanup()
		return trace.ConvertSystemError(err)
	}
	return nil
}

func ConvertError(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	statusErr, ok := err.(*errors.StatusError)
	if !ok {
		return err
	}

	message := fmt.Sprintf("%v", err)
	if !isEmptyDetails(statusErr.ErrStatus.Details) {
		message = fmt.Sprintf("%v, details: %v", message, statusErr.ErrStatus.Details)
	}
	if format != "" {
		message = fmt.Sprintf("%v: %v", fmt.Sprintf(format, args...), message)
	}

	status := statusErr.Status()
	switch {
	case status.Code == http.StatusConflict && status.Reason == metav1.StatusReasonAlreadyExists:
		return trace.AlreadyExists(message)
	case status.Code == http.StatusNotFound:
		return trace.NotFound(message)
	case status.Code == http.StatusForbidden:
		return trace.AccessDenied(message)
	}
	return err
}

func isEmptyDetails(details *metav1.StatusDetails) bool {
	if details == nil {
		return true
	}

	if details.Name == "" && details.Group == "" && details.Kind == "" && len(details.Causes) == 0 {
		return true
	}
	return false
}
