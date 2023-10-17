package util

import (
	"crypto/x509"
	"encoding/pem"
	"log"
)

func GetCNFromCert(certRaw []byte) string {
	block, rest := pem.Decode(certRaw)
	if block == nil {
		log.Fatal("failed to parse PEM block containing the certificate")
	}
	for block.Type != "CERTIFICATE" && block != nil {
		block, rest = pem.Decode(rest)
	}

	cert, err := x509.ParseCertificate(block.Bytes) // handle error
	if err != nil {
		log.Fatalf("error decoding PEM cert: %s", err)
	}
	return cert.Subject.CommonName
}
