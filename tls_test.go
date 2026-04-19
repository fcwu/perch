package main

import (
	"crypto/tls"
	"crypto/x509"
	"testing"
)

func TestGenerateSelfSignedCert(t *testing.T) {
	certPEM, keyPEM, err := generateSelfSignedCert("perch-ca", nil, nil)
	if err != nil {
		t.Fatalf("generateSelfSignedCert: %v", err)
	}
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		t.Fatal("cert or key is empty")
	}
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}
	leaf, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	if leaf.Subject.CommonName != "perch-ca" {
		t.Errorf("expected CN=perch-ca, got %s", leaf.Subject.CommonName)
	}
}
