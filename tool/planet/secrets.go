package main

import (
	"os"

	"github.com/gravitational/trace"
)

// initSecrets takes directory and initializes k8s secrets like
// TLS Certificate authority and APIserver certificate in it
func initSecrets(dir, domain string, serviceSubnet CIDR) error {
	if err := os.MkdirAll(dir, 0777); err != nil {
		return trace.Wrap(err)
	}

	// init key pair for certificate authority
	ca, err := initKeyPair(
		dir, domain, CertificateAuthorityKeyPair, serviceSubnet, nil, true)
	if err != nil {
		return trace.Wrap(err)
	}

	// init key pair for apiserver signed by our authority
	_, err = initKeyPair(
		dir, domain, APIServerKeyPair, serviceSubnet, ca.keyPair, false)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}
