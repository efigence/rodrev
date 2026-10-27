package util

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
)

// GetCNFromCert returns the CN of the first certificate in PEM data. The data
// can carry other blocks too (a key bundled with its cert is common), those
// are skipped
func GetCNFromCert(certRaw []byte) (string, error) {
	var found []string
	rest := certRaw
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			found = append(found, block.Type)
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return "", fmt.Errorf("CERTIFICATE block can not be parsed: %s", err)
		}
		return cert.Subject.CommonName, nil
	}
	if len(found) == 0 {
		return "", fmt.Errorf("no PEM data found, expected a CERTIFICATE block")
	}
	return "", fmt.Errorf("no CERTIFICATE block in PEM data, only: %s", strings.Join(found, ", "))
}
