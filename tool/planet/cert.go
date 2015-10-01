package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
)

// keyPair is struct holding private key and a template for
// public certificate. It has to be RSA because this is what K8s works with
type keyPair struct {
	priv     *rsa.PrivateKey
	template x509.Certificate
}

func newKeyPair(c *Config, ca bool) (*keyPair, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(time.Second * (86400 * 120)) // 120 days

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Gravitational"},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageDataEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	if ca {
		template.IsCA = true
		template.KeyUsage |= x509.KeyUsageCertSign
	}

	if ip := net.ParseIP(c.MasterIP); ip != nil {
		template.IPAddresses = append(template.IPAddresses, ip)
	} else {
		template.DNSNames = append(template.DNSNames, c.MasterIP)
	}
	// ServiceSubnet is the subnet of the services run by k8s
	// the first IP is usually given to the first service that starts up
	// in this subnet - k8s API service
	template.IPAddresses = append(
		template.IPAddresses, c.ServiceSubnet.FirstIP())

	return &keyPair{
		priv:     priv,
		template: template,
	}, nil
}

// writeCertificate will generate a certificate and output it to a writer.
// if parent is nil, the certificate will be self-signed, otherwise it will
// be signed by the parent that should be an authority
func (k *keyPair) writeCertificate(parent *keyPair, w io.Writer) error {
	var signerTemplate x509.Certificate
	var signerPrivateKey interface{}
	if parent == nil { // this is self signed certificate
		signerTemplate = k.template
		signerPrivateKey = k.priv
	} else { // this is signed by authority
		signerTemplate = parent.template
		signerPrivateKey = parent.priv
	}

	derBytes, err := x509.CreateCertificate(
		rand.Reader,
		&k.template,      // template of the certificate
		&signerTemplate,  // template of the CA certificate
		k.publicKey(),    // public key of the signee
		signerPrivateKey) // private key of the signer
	if err != nil {
		return trace.Wrap(err)
	}

	if err := pem.Encode(w, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func (k *keyPair) publicKey() interface{} {
	return &k.priv.PublicKey
}

func (k *keyPair) writePrivateKey(w io.Writer) error {
	if err := pem.Encode(w, pemBlockForKey(k.priv)); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func pemBlockForKey(priv interface{}) *pem.Block {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(k),
		}
	default:
		return nil
	}
}

// keyPairPaths helps to check key pairs on local disk and
// initialize them if necessary
type keyPairPaths struct {
	sourceDir string
	name      string
	keyPair   *keyPair
}

func (k *keyPairPaths) certPath() string {
	return filepath.Join(k.sourceDir, fmt.Sprintf("%v.cert", k.name))
}

func (k *keyPairPaths) keyPath() string {
	return filepath.Join(k.sourceDir, fmt.Sprintf("%v.key", k.name))
}

func (k *keyPairPaths) exists() (bool, error) {
	var haveKey bool
	if _, err := os.Stat(k.keyPath()); err != nil {
		if !os.IsNotExist(err) {
			return false, trace.Wrap(err)
		}
		haveKey = false
	}

	var haveCert bool
	if _, err := os.Stat(k.certPath()); err != nil {
		if !os.IsNotExist(err) {
			return false, trace.Wrap(err)
		}
		haveCert = false
	}

	// check for strange situation where cert or key are missing and report
	// an error
	if (!haveCert && haveKey) || (haveCert && !haveKey) {
		return false, trace.Errorf(
			"either cert or key are missing, and the other is present. fix the issue by recovering both or deleting both")
	}

	return haveCert && haveKey, nil
}

func (k *keyPairPaths) remove() error {
	var err error
	err = os.Remove(k.certPath())
	err = os.Remove(k.keyPath())
	return err
}

func (k *keyPairPaths) write(parent *keyPair) error {
	fkey, err := os.Create(k.keyPath())
	if err != nil {
		return trace.Wrap(err)
	}
	defer fkey.Close()

	fcert, err := os.Create(k.certPath())
	if err != nil {
		defer k.remove()
		return trace.Wrap(err)
	}
	defer fcert.Close()

	if err := k.keyPair.writePrivateKey(fkey); err != nil {
		return err
	}
	if err := k.keyPair.writeCertificate(parent, fcert); err != nil {
		defer k.remove()
		return err
	}
	return nil
}

func initKeyPair(c *Config, name string, parent *keyPair, ca bool) (*keyPairPaths, error) {
	p := &keyPairPaths{
		name:      name,
		sourceDir: c.StateDir,
	}
	// key pair have been already initialized
	exists, err := p.exists()
	if err != nil {
		return nil, err
	}
	if exists {
		// TODO(klizhentas) should read the keyPair actually if it exists
		return p, nil
	}

	// generate and write key pairs
	kp, err := newKeyPair(c, ca)
	if err != nil {
		return nil, err
	}
	p.keyPair = kp

	if err := p.write(parent); err != nil {
		return nil, err
	}
	return p, nil
}
