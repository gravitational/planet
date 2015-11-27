package monitoring

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
)

type (
	monit struct {
		client *http.Client
	}

	service struct {
		Name      string
		ErrorMask serviceErrorMask
		ErrorHint int
		Pid       int
	}

	serviceErrorMask int

	// valueNode is an XML node with no attributes and a value
	valueNode struct {
		Value string `xml:",chardata"`
	}

	serviceType byte
)

// Service types
// https://mmonit.com/monit/documentation/monit.html
// (The Monit Control File)
const (
	Filesystem serviceType = 0 // Device/disk, mount point, file or a directory
	Directory              = 1
	File                   = 2
	Process                = 3 // Running process
	Host                   = 4 // Hostname / IP-address
	System                 = 5 // General system resources (CPU usage, total memory usage or load average)
	Fifo                   = 6 // FIFO named pipe (http://man7.org/linux/man-pages/man7/fifo.7.html)
	Program                = 7 // Executable program or script
	Net                    = 8 // IPv4/IPv6 network interface address
)

// Error conditions (monit events)
const (
	ErrorConnection = 0x20  // Host check failure
	ErrorNotRunning = 0x200 // Process has not started
)

const socketPath = "/etc/monit/sock"

func newMonitService() Interface {
	client := &http.Client{
		Transport: &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	}

	return &monit{client: client}
}

func (r *monit) Status() ([]ServiceStatus, error) {
	if !isSocketReady() {
		return nil, ErrMonitorNotReady
	}

	resp, err := r.client.Get("http://monit/_status?format=xml")
	if err != nil {
		return nil, trace.Wrap(err)
	}
	defer resp.Body.Close()

	var services []*service
	services, err = parse(resp.Body)
	if err != nil {
		return nil, trace.Wrap(err, "invalid monit status response")
	}

	var conditions []ServiceStatus
	for _, svc := range services {
		if svc.ErrorMask == 0 {
			continue
		}

		conditions = append(conditions, ServiceStatus{
			Name:    svc.Name,
			Status:  StatusFailed,
			Message: "monit: " + svc.ErrorMask.String(),
		})
	}

	return conditions, nil
}

// parse parses monit state provided as XML in rdr and returns a list of services
func parse(rdr io.Reader) ([]*service, error) {
	decoder := xml.NewDecoder(rdr)
	decoder.Strict = false
	// Monit status is returned as ISO-8859-1 encoded XML
	decoder.CharsetReader = selectReader

	var (
		token    xml.Token
		err      error
		services []*service
	)

	for {
		if token, err = decoder.Token(); err != nil {
			if err == io.EOF {
				return services, nil
			}
			return nil, trace.Wrap(err)
		}
		switch elem := token.(type) {
		case xml.StartElement:
			if elem.Name.Local == "service" && isServiceType(elem, Process) {
				svc := &service{}
				if err = decoder.DecodeElement(svc, &elem); err != nil {
					return nil, trace.Wrap(err)
				}
				services = append(services, svc)
			}
		}
	}
}

func (r *service) UnmarshalXML(decoder *xml.Decoder, node xml.StartElement) error {
	var token xml.Token
	var err error

	for {
		if token, err = decoder.Token(); err != nil {
			if err == io.EOF {
				return nil
			}
			return trace.Wrap(err)
		}
		switch elem := token.(type) {
		case xml.StartElement:
			var value valueNode

			switch elem.Name.Local {
			case "name", "status", "status_hint", "pid":
				if err = decoder.DecodeElement(&value, &elem); err != nil {
					return trace.Wrap(err)
				}
			}
			switch elem.Name.Local {
			case "name":
				r.Name = value.Value
			case "status":
				r.ErrorMask = serviceErrorMask(parseInt(value.Value))
			case "status_hint":
				r.ErrorHint = parseInt(value.Value)
			case "pid":
				r.Pid = parseInt(value.Value)
			}
		}
	}
}

func (r *service) String() string {
	return fmt.Sprintf("service(name=%s, error=%s, pid=%d)", r.Name, r.ErrorMask, r.Pid)
}

func (r serviceErrorMask) String() string {
	var errors []string
	const knownErrorsMask = ErrorNotRunning | ErrorConnection

	if r == 0 {
		return "no error"
	}
	if r&ErrorNotRunning != 0 {
		errors = append(errors, "not running")
	}
	if r&ErrorConnection != 0 {
		errors = append(errors, "failed healthz check")
	}
	if r&^knownErrorsMask != 0 {
		errors = append(errors, strconv.FormatUint(uint64(r&^knownErrorsMask), 10))
	}

	return strings.Join(errors, ",")
}

func isServiceType(elem xml.StartElement, serviceType serviceType) bool {
	for _, attr := range elem.Attr {
		if attr.Name.Local == "type" && attr.Value == strconv.FormatUint(uint64(serviceType), 10) {
			return true
		}
	}
	return false
}

func parseInt(value string) int {
	result, err := strconv.ParseInt(value, 10, 32)

	if err != nil {
		log.Errorf("invalid numeric value %s: %v", value, err)
		// Fall-through with zero value
	}
	return int(result)
}

// charsetReader is an io.Reader that reads ISO-8859-1 encoded data
type charsetReader struct {
	rdr io.ByteReader
	buf bytes.Buffer
}

func (r *charsetReader) ReadByte() (byte, error) {
	if r.buf.Len() == 0 {
		b, err := r.rdr.ReadByte()
		if err != nil {
			return 0, err
		}
		if b < utf8.RuneSelf {
			return b, nil
		}
		r.buf.WriteRune(rune(b))
	}
	return r.buf.ReadByte()
}

// charsetReader only implements io.ByteReader
func (r *charsetReader) Read(p []byte) (int, error) {
	return 0, io.EOF
}

// selectReader creates an ISO-8859-1 capable reader for input if charset == ISO-8859-1,
// otherwise it returns input unmodified
func selectReader(charset string, input io.Reader) (io.Reader, error) {
	switch strings.ToUpper(charset) {
	case "ISO-8859-1":
		return &charsetReader{rdr: input.(io.ByteReader)}, nil
	default:
		return input, nil
	}
}

func isSocketReady() bool {
	_, err := os.Stat(socketPath)
	return err == nil
}
