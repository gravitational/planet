package monitoring

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
)

type (
	monit struct {
		socket string
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

func New() (Service, error) {
	return &monit{
		socket: "/etc/monit/sock", // FIXME: configure
	}, nil
}

func (r *monit) Status() (conditions []Status, err error) {
	var (
		resp     *http.Response
		services []*service
	)

	client := &http.Client{
		Transport: &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				return net.Dial("unix", r.socket)
			},
		},
	}

	resp, err = client.Get("http://monit/_status?format=xml")
	if err != nil {
		return nil, trace.Wrap(err)
	}
	defer resp.Body.Close()

	services, err = r.parse(resp.Body)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	for _, svc := range services {
		state := State_Running

		if svc.Error != 0 {
			state = State_Failed
		}

		conditions = append(conditions, Status{
			Module:    svc.Name,
			Timestamp: time.Now(), // FIXME: actual timestamp
			State:     state,
			Message:   svc.Error.String(),
		})
	}

	return conditions, nil
}

func (r *monit) parse(rdr io.Reader) ([]*service, error) {
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

func (r *service) UnmarshalXML(decoder *xml.Decoder, node xml.StartElement) (err error) {
	var (
		token xml.Token
	)
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
	if err != nil {
		decoder.Skip()
	}
	return err
}

func (r *service) String() string {
	return fmt.Sprintf("service(name=%s, error=%d, pid=%d)", r.Name, r.Error, r.Pid)
}

func (r serviceError) String() string {
	switch r {
	case 0x20:
		return "failed healthz check"
	case 0x200:
		return "not running"
	default:
		return strconv.FormatUint(uint64(r), 10)
	}
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
