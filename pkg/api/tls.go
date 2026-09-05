package api

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"

	fs "github.com/fmjstudios/gopskit/pkg/fsi"
)

func LoadX509KeyPair(cert, key string) (tls.Certificate, error) {
	cexists := fs.CheckIfExists(cert)
	if !cexists {
		return tls.Certificate{}, fmt.Errorf("cannot parse certificate: %s. file not found", cert)
	}

	kexists := fs.CheckIfExists(key)
	if !kexists {
		return tls.Certificate{}, fmt.Errorf("cannot parse private key: %s. file not found", key)
	}

	pair, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("cannot load certificate key pair: %s/%s. Error: %v", cert, key, err)
	}

	return pair, nil
}

func LoadCACertPool(paths ...string) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	for _, path := range paths {
		exists := fs.CheckIfExists(path)
		if !exists {
			return nil, fmt.Errorf("cannot add certificate: %s to pool. file not found", path)
		}

		fc, err := fs.Read(path)
		if err != nil {
			return nil, err
		}

		pool.AppendCertsFromPEM(fc)
	}

	return pool, nil
}
