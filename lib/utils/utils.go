package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"reflect"
	"sort"
	"strings"
	"syscall"

	"github.com/gravitational/trace"

	log "github.com/Sirupsen/logrus"
)

// UpsertHostsLines either updates an existing hosts entry or inserts a new
// entry with provided hostnames and IPs
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
		hostnames := strings.Fields(e.Hostnames)
		sourceHostnames := append([]string{}, fields[1:]...)
		if compareStringSlices(sourceHostnames, hostnames) {
			fields[0] = e.IP
			return strings.Join(fields, " "), append(entries[:i], entries[i+1:]...)
		}
	}

	return line, entries
}

// HandleSignals configures signal handling for the process.
// It configures two groups of signals: ignored and terminal.
// Upon receiving any of the terminal signals, it invokes the
// provided function prior to exit.
func HandleSignals(fn func() error) error {
	c := SetupSignalHandler()
	select {
	case sig := <-c:
		log.Infof("received a %s signal, stopping...", sig)
		err := fn()
		if err != nil {
			log.Errorf("handler failed: %v", err)
		}
		return trace.Wrap(err)
	}
	return nil
}

// SetupSignalHandler configures a set of ignored and termination signals.
// Returns a channel to receive notifications about termination signals.
func SetupSignalHandler() (recvCh <-chan os.Signal) {
	var ignores = []os.Signal{
		syscall.SIGPIPE, syscall.SIGHUP,
		syscall.SIGUSR1, syscall.SIGUSR2,
		syscall.SIGALRM,
	}
	var terminals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT}
	recvCh = make(chan os.Signal, 1)
	signal.Ignore(ignores...)
	signal.Notify(recvCh, terminals...)
	return recvCh
}

func compareStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sort.Sort(sort.StringSlice(a))
	sort.Sort(sort.StringSlice(b))
	return reflect.DeepEqual(a, b)
}
