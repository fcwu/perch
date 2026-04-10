package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"

	"software.sslmate.com/src/go-pkcs12"
)

type BootstrapHandler struct {
	p12Data  []byte
	usedFile string
	used     atomic.Bool
}

// newBootstrapHandler creates the handler. usedFile is the path to the
// sentinel file that persists the "already used" state across restarts.
// If the file already exists, bootstrap is immediately disabled.
func newBootstrapHandler(p12Data []byte, usedFile string) *BootstrapHandler {
	bh := &BootstrapHandler{p12Data: p12Data, usedFile: usedFile}
	if _, err := os.Stat(usedFile); err == nil {
		bh.used.Store(true)
	}
	return bh
}

func (b *BootstrapHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !b.used.CompareAndSwap(false, true) {
		http.Error(w, "bootstrap already used", http.StatusGone)
		return
	}
	// Persist the used state before sending the response so a crash mid-write
	// cannot allow a second download.
	if b.usedFile != "" {
		os.MkdirAll(filepath.Dir(b.usedFile), 0700)
		os.WriteFile(b.usedFile, []byte("used"), 0600)
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
