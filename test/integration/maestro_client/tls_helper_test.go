package maestro_client_integration

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// TLSTestCerts holds all PEM-encoded certificates and keys for TLS integration tests.
type TLSTestCerts struct {
	CACert     []byte
	CAKey      []byte
	ServerCert []byte
	ServerKey  []byte
	ClientCert []byte
	ClientKey  []byte
	TempDir    string
}

// generateTestCerts creates a self-signed CA, server cert (with localhost SANs),
// and client cert for integration testing.
func generateTestCerts() (*TLSTestCerts, error) {
	certs := &TLSTestCerts{}

	// --- CA ---
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"HyperFleet Test CA"},
			CommonName:   "Test CA",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}

	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		return nil, err
	}

	certs.CACert = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})
	caKeyDER, err := x509.MarshalECPrivateKey(caKey)
	if err != nil {
		return nil, err
	}
	certs.CAKey = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: caKeyDER})

	// --- Server cert (signed by CA, SANs: localhost, 127.0.0.1, 0.0.0.0) ---
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization: []string{"HyperFleet Test Server"},
			CommonName:   "localhost",
		},
		NotBefore:   time.Now().Add(-1 * time.Hour),
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{"localhost"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("0.0.0.0")},
	}

	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}

	certs.ServerCert = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCertDER})
	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		return nil, err
	}
	certs.ServerKey = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyDER})

	// --- Client cert (signed by CA) ---
	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject: pkix.Name{
			Organization: []string{"HyperFleet Test Client"},
			CommonName:   "test-client",
		},
		NotBefore:   time.Now().Add(-1 * time.Hour),
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	clientCertDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caCert, &clientKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}

	certs.ClientCert = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientCertDER})
	clientKeyDER, err := x509.MarshalECPrivateKey(clientKey)
	if err != nil {
		return nil, err
	}
	certs.ClientKey = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: clientKeyDER})

	return certs, nil
}

// WriteToTempDir writes all cert/key files to a temp directory for mounting into containers.
func (c *TLSTestCerts) WriteToTempDir() error {
	dir, err := os.MkdirTemp("", "maestro-tls-test-*")
	if err != nil {
		return err
	}
	c.TempDir = dir

	files := map[string][]byte{
		"ca.crt":     c.CACert,
		"server.crt": c.ServerCert,
		"server.key": c.ServerKey,
		"client.crt": c.ClientCert,
		"client.key": c.ClientKey,
	}

	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			return err
		}
	}

	return nil
}

// Cleanup removes the temp directory.
func (c *TLSTestCerts) Cleanup() {
	if c.TempDir != "" {
		_ = os.RemoveAll(c.TempDir)
	}
}

// CAFilePath returns the path to the CA cert file on host.
func (c *TLSTestCerts) CAFilePath() string {
	return filepath.Join(c.TempDir, "ca.crt")
}

// ClientCertFilePath returns the path to the client cert file on host.
func (c *TLSTestCerts) ClientCertFilePath() string {
	return filepath.Join(c.TempDir, "client.crt")
}

// ClientKeyFilePath returns the path to the client key file on host.
func (c *TLSTestCerts) ClientKeyFilePath() string {
	return filepath.Join(c.TempDir, "client.key")
}
