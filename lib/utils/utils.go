package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
)

// UpsertHostsLine updates the existing hosts entry or inserts a new
// entry with hostname and ip
func UpsertHostsLine(reader io.Reader, writer io.Writer, hostname, ip string) error {
	scanner := bufio.NewScanner(reader)
	var replacedEntry bool
	for scanner.Scan() {
		replaced, line := replaceLine(scanner.Text(), hostname, ip)
		if replaced {
			replacedEntry = true
		}
		if _, err := io.WriteString(writer, line+"\n"); err != nil {
			return trace.Wrap(err)
		}
	}
	if replacedEntry {
		return nil
	}
	entry := fmt.Sprintf("%v %v", ip, hostname)
	if _, err := io.WriteString(writer, entry+"\n"); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// UpsertHostsFile modifies /etc/hosts to contain hostname and ip
// pair, either by adding new entry or replacing the existing one
// in a given path, if path is empty, it will be "/etc/hosts"
func UpsertHostsFile(hostname, ip string, path string) error {
	if path == "" {
		path = "/etc/hosts"
	}
	fi, err := os.Stat(path)
	if err != nil {
		return trace.Wrap(err)
	}
	contents, err := ioutil.ReadFile(path)
	if err != nil {
		return trace.Wrap(err)
	}
	input := bytes.NewBuffer(contents)
	output := &bytes.Buffer{}
	if err := UpsertHostsLine(input, output, hostname, ip); err != nil {
		return trace.Wrap(err)
	}
	err = ioutil.WriteFile(path, output.Bytes(), fi.Mode())
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func replaceLine(line, hostname, ip string) (bool, string) {
	if strings.HasPrefix(line, "#") {
		return false, line
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return false, line
	}
	if fields[1] != hostname {
		return false, line
	}
	fields[0] = ip
	return true, strings.Join(fields, " ")
}
