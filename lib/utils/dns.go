// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin dragonfly freebsd linux netbsd openbsd solaris

// Read system DNS config from /etc/resolv.conf

package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"

	"github.com/gravitational/trace"
	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
)

var defaultNS = []string{"127.0.0.1", "::1"}

type DNSConfig struct {
	Servers    []string // servers to use
	Domain     string   // Domain parameter
	Search     []string // suffixes to append to local name
	Ndots      int      // number of dots in name to trigger absolute lookup
	Timeout    int      // seconds before giving up on packet
	Attempts   int      // lost packets before giving up on server
	Rotate     bool     // round robin among servers
	UnknownOpt bool     // anything unknown was encountered
	Lookup     []string // OpenBSD top-level database "lookup" order
}

func (d *DNSConfig) ndots() string {
	return fmt.Sprintf("%v%v", ndotsPrefix, d.Ndots)
}

func (d *DNSConfig) timeout() string {
	return fmt.Sprintf("%v%v", timeoutPrefix, d.Timeout)
}

func (d *DNSConfig) attempts() string {
	return fmt.Sprintf("%v%v", attemptsPrefix, d.Attempts)
}

func (d *DNSConfig) rotate() string {
	if d.Rotate {
		return " " + rotateParam
	}
	return ""
}

// String returns resolv.conf serialized version of config
func (d *DNSConfig) String() string {
	buf := &bytes.Buffer{}
	if d.Domain != "" {
		fmt.Fprintf(buf, "%v %v\n", domainParam, d.Domain)
	}
	search := []string{}
	for _, domain := range d.Search {
		if domain != d.Domain {
			search = append(search, domain)
		}
	}
	if len(search) != 0 {
		fmt.Fprintf(buf, "%v %v\n", searchParam, strings.Join(search, " "))
	}
	for _, server := range d.Servers {
		fmt.Fprintf(buf, "%v %v\n", nameserverParam, server)
	}
	fmt.Fprintf(buf, "%v %v %v %v%v\n",
		optionsParam, d.ndots(), d.timeout(), d.attempts(), d.rotate())
	if len(d.Lookup) != 0 {
		fmt.Fprintf(buf, "%v %v\n", lookupParam, strings.Join(d.Lookup, " "))
	}
	return buf.String()
}

// See resolv.conf(5) on a Linux machine.
// TODO(rsc): Supposed to call uname() and chop the beginning
// of the host name to get the default search domain.
func DNSReadConfig(rdr io.Reader) (*DNSConfig, error) {
	conf := &DNSConfig{
		Ndots:    1,
		Timeout:  5,
		Attempts: 2,
	}

	scanner := bufio.NewScanner(rdr)

	for scanner.Scan() {
		line := scanner.Text()

		if len(line) > 0 && (line[0] == ';' || line[0] == '#') {
			// comment.
			continue
		}
		f := strings.Fields(line)
		if len(f) < 1 {
			continue
		}
		switch f[0] {
		case nameserverParam: // add one name server
			if len(f) > 1 && len(conf.Servers) < 3 { // system limit
				conf.Servers = append(conf.Servers, f[1])
			}

		case domainParam: // set search path to just this domain
			if len(f) > 1 {
				conf.Domain = f[1]
				conf.Search = []string{f[1]}
			}

		case searchParam: // set search path to given servers
			conf.Search = make([]string, len(f)-1)
			for i := 0; i < len(conf.Search); i++ {
				conf.Search[i] = f[i+1]
			}

		case optionsParam: // magic options
			for _, s := range f[1:] {
				switch {
				case strings.HasPrefix(s, ndotsPrefix):
					n, _ := strconv.Atoi(s[len(ndotsPrefix):])
					if n < 1 {
						n = 1
					}
					conf.Ndots = n
				case strings.HasPrefix(s, timeoutPrefix):
					n, _ := strconv.Atoi(s[len(timeoutPrefix):])
					if n < 1 {
						n = 1
					}
					conf.Timeout = n
				case strings.HasPrefix(s, attemptsPrefix):
					n, _ := strconv.Atoi(s[len(attemptsPrefix):])
					if n < 1 {
						n = 1
					}
					conf.Attempts = n
				case s == rotateParam:
					conf.Rotate = true
				default:
					conf.UnknownOpt = true
				}
			}

		case lookupParam:
			// OpenBSD option:
			// http://www.openbsd.org/cgi-bin/man.cgi/OpenBSD-current/man5/resolv.conf.5
			// "the legal space-separated values are: bind, file, yp"
			conf.Lookup = f[1:]

		default:
			conf.UnknownOpt = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(conf.Servers) == 0 {
		conf.Servers = append(conf.Servers, defaultNS...)
	}

	return conf, nil
}

// ResolveAddr resolves the provided hostname using the local resolver
func ResolveAddr(addr string) (hostPort string, err error) {
	host := addr
	port := ""
	if strings.Contains(addr, ":") {
		host, port, err = net.SplitHostPort(addr)
		if err != nil {
			return "", trace.Wrap(err)
		}
	}
	if ip := net.ParseIP(host); len(ip) == 0 {
		c := dns.Client{}
		m := dns.Msg{}
		m.SetQuestion(host+".", dns.TypeA)
		r, t, err := c.Exchange(&m, localResolverAddr)
		if err != nil {
			return "", trace.Wrap(err)
		}
		log.Debugf("resolve %v took %v", host, t)
		if len(r.Answer) == 0 {
			return "", trace.ConnectionProblem(nil, "failed to resolve %v", addr)
		}
		for _, answer := range r.Answer {
			record := answer.(*dns.A)
			log.Debugf("resolved %v to %v", host, record.A)
			host = record.A.String()
			break
		}
	}
	if port != "" {
		host = fmt.Sprintf("%v:%v", host, port)
	}
	return host, nil
}

const (
	ndotsPrefix     = "ndots:"
	timeoutPrefix   = "timeout:"
	attemptsPrefix  = "attempts:"
	rotateParam     = "rotate"
	lookupParam     = "lookup"
	nameserverParam = "nameserver"
	domainParam     = "domain"
	searchParam     = "search"
	optionsParam    = "options"

	localResolverAddr = "127.0.0.1:53"
)
