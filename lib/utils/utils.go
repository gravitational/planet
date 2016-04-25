package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/gravitational/trace"
)

// UpsertHostsLines updates the existing hosts entry or inserts a new
// entry with hostnames and ips
func UpsertHostsLines(reader io.Reader, writer io.Writer, entries []HostEntry) error {
	remainingEntries := make([]HostEntry, len(entries))
	copy(remainingEntries, entries)
	scanner := bufio.NewScanner(reader)
	var line string
	for scanner.Scan() {
		// replaceLine will remove the host entry from the entries list
		// if it finds the match
		line, remainingEntries = replaceLine(scanner.Text(), remainingEntries)
		if _, err := io.WriteString(writer, line+"\n"); err != nil {
			return trace.Wrap(err)
		}
	}
	// just append remaining entries if there was no match found
	for _, e := range remainingEntries {
		line := fmt.Sprintf("%v %v", e.IP, e.Hostnames)
		if _, err := io.WriteString(writer, line+"\n"); err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

// HostEntry consists of host name and IP it resolves to
type HostEntry struct {
	// Hostnames is a list of space separated hostnames
	Hostnames string
	// IP is the IP the hostnames should resolve to
	IP string
}

// UpsertHostsFile modifies /etc/hosts to contain hostname and ip
// entries, either by adding new entry or replacing the existing one
// in a given path, if path is empty, it will be "/etc/hosts"
func UpsertHostsFile(entries []HostEntry, path string) error {
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
	if err := UpsertHostsLines(input, output, entries); err != nil {
		return trace.Wrap(err)
	}
	err = ioutil.WriteFile(path, output.Bytes(), fi.Mode())
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func replaceLine(line string, entries []HostEntry) (string, []HostEntry) {
	if strings.HasPrefix(line, "#") {
		return line, entries
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return line, entries
	}
	for i, e := range entries {
		if fields[1] == e.Hostnames {
			fields[0] = e.IP
			return strings.Join(fields, " "), append(entries[:i], entries[i+1:]...)
		}
	}

	return line, entries
}
