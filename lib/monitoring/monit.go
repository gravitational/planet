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

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
)

type (
	monit struct {
		client *http.Client
	}

	service struct {
		Name      string
		Error     serviceError
		ErrorHint serviceErrorHint
		Pid       uint
	}

	serviceError     uint
	serviceErrorHint uint

	valueNode struct {
		Value string `xml:",chardata"`
	}

	serviceType byte
)

const (
	Filesystem serviceType = 0
	Directory              = 1
	File                   = 2
	Daemon                 = 3
	Connection             = 4
	System                 = 5
)

// monit events
const (
	ErrorConnection = 0x20
	ErrorNotRunning = 0x200
)

const socket = "/etc/monit/sock"

func newMonitService() Interface {
	client := &http.Client{
		Transport: &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				return net.Dial("unix", socket)
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
		if svc.Error == 0 {
			continue
		}

		conditions = append(conditions, ServiceStatus{
			Name:    svc.Name,
			State:   StateFailed,
			Message: "monit: " + svc.Error.String(),
		})
	}

	return conditions, nil
}

func parse(rdr io.Reader) ([]*service, error) {
	var (
		token    xml.Token
		err      error
		services []*service
	)
	decoder := xml.NewDecoder(rdr)
	decoder.Strict = false
	decoder.CharsetReader = readerISO8859_1

L:
	for {
		if token, err = decoder.Token(); err != nil {
			if err == io.EOF {
				err = nil
			}
			break L
		}
		switch elem := token.(type) {
		case xml.StartElement:
			if elem.Name.Local == "service" && isServiceType(elem, Daemon) {
				svc := &service{}
				if err = decoder.DecodeElement(svc, &elem); err != nil {
					return nil, trace.Wrap(err)
				}
				services = append(services, svc)
			}
		}
	}

	if err != nil {
		err = trace.Wrap(err)
	}

	return services, err
}

func (r *service) UnmarshalXML(decoder *xml.Decoder, node xml.StartElement) error {
	var token xml.Token
	var err error
L:
	for {
		if token, err = decoder.Token(); err != nil {
			if err == io.EOF {
				err = nil
			}
			break L
		}
		switch elem := token.(type) {
		case xml.StartElement:
			var value valueNode

			switch elem.Name.Local {
			case "name", "status", "status_hint", "pid":
				if err = decoder.DecodeElement(&value, &elem); err != nil {
					break L
				}
			}
			switch elem.Name.Local {
			case "name":
				r.Name = value.Value
			case "status":
				r.Error = serviceError(mustParseUint(value.Value))
			case "status_hint":
				r.ErrorHint = serviceErrorHint(mustParseUint(value.Value))
			case "pid":
				r.Pid = mustParseUint(value.Value)
			}
		}
	}
	return err
}

func (r *service) String() string {
	return fmt.Sprintf("service(name=%s, error=%s, pid=%d)", r.Name, r.Error, r.Pid)
}

func (r serviceError) String() string {
	var errors []string
	const errorMask = ErrorNotRunning | ErrorConnection

	if r == 0 {
		return "no error"
	}
	if r&ErrorNotRunning != 0 {
		errors = append(errors, "not running")
	}
	if r&ErrorConnection != 0 {
		errors = append(errors, "failed healthz check")
	}
	if r&^errorMask != 0 {
		errors = append(errors, strconv.FormatUint(uint64(r&^errorMask), 10))
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

func mustParseUint(value string) uint {
	result, err := strconv.ParseUint(value, 10, 32)

	if err != nil {
		panic(err)
	}
	return uint(result)
}

type charsetReader struct {
	r   io.ByteReader
	buf bytes.Buffer
}

func (r *charsetReader) ReadByte() (byte, error) {
	var (
		b   byte
		err error
	)

	if r.buf.Len() == 0 {
		b, err = r.r.ReadByte()
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

func (r *charsetReader) Read(p []byte) (int, error) {
	return 0, io.EOF
}

func readerISO8859_1(charset string, r io.Reader) (io.Reader, error) {
	return &charsetReader{r: r.(io.ByteReader)}, nil
}

func isSocketReady() bool {
	_, err := os.Stat(socket)
	return err == nil
}
