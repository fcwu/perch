package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"sync/atomic"

	"software.sslmate.com/src/go-pkcs12"
)

type BootstrapHandler struct {
	p12Data []byte
	used    atomic.Bool
}

func newBootstrapHandler(p12Data []byte) *BootstrapHandler {
	return &BootstrapHandler{p12Data: p12Data}
}

func (b *BootstrapHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !b.used.CompareAndSwap(false, true) {
		http.Error(w, "bootstrap already used", http.StatusGone)
		return
	}
	w.Header().Set("Content-Type", "application/x-pkcs12")
	w.Header().Set("Content-Disposition", `attachment; filename="client.p12"`)
	w.Write(b.p12Data)
}

func generateClientP12(caCertPEM, caKeyPEM []byte, password string) ([]byte, error) {
	caBlock, _ := pem.Decode(caCertPEM)
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return nil, err
	}

	clientCertPEM, clientKeyPEM, err := generateSelfSignedCert("perch-client", caCert)
	if err != nil {
		return nil, err
	}

	tlsCert, err := tls.X509KeyPair(clientCertPEM, clientKeyPEM)
	if err != nil {
		return nil, err
	}
	leaf, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return nil, err
	}
	keyBlock, _ := pem.Decode(clientKeyPEM)
	privKey, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, err
	}
	return pkcs12.Legacy.Encode(privKey, leaf, []*x509.Certificate{caCert}, password)
}
