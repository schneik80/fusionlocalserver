package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/schneik80/fusionlocalserver/config"
)

// TLS cert/key basenames cached under config.Dir() when -tls is used without a
// caller-supplied cert.
const (
	tlsCertFile = "tls-cert.pem"
	tlsKeyFile  = "tls-key.pem"
)

// resolveTLSPaths returns the cert and key paths for a TLS run. A
// caller-supplied pair (cert and key both set) is used as-is; otherwise a
// self-signed pair is generated once and cached under config.Dir(), then
// reused. Browsers warn on the self-signed cert — that's expected on a LAN
// without a real CA; the point is to encrypt the wire so the session cookie can
// carry the Secure flag.
func resolveTLSPaths(cert, key string) (certPath, keyPath string, selfSigned bool, err error) {
	if cert != "" && key != "" {
		return cert, key, false, nil
	}
	dir, err := config.Dir()
	if err != nil {
		return "", "", false, err
	}
	certPath = filepath.Join(dir, tlsCertFile)
	keyPath = filepath.Join(dir, tlsKeyFile)
	if err := ensureSelfSignedCert(certPath, keyPath); err != nil {
		return "", "", false, err
	}
	return certPath, keyPath, true, nil
}

// ensureSelfSignedCert writes a self-signed cert+key to the given paths unless
// both already exist. The cert covers localhost, the loopback addresses, the
// machine's hostname, and its non-loopback IPv4 interface addresses, so a
// browser reaching the server by any of those validates the hostname (after the
// one-time "untrusted issuer" click-through).
func ensureSelfSignedCert(certPath, keyPath string) error {
	if fileExists(certPath) && fileExists(keyPath) {
		return nil
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generating key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("generating serial: %w", err)
	}

	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "fusionlocalserver"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(5, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              certDNSNames(),
		IPAddresses:           certIPs(),
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return fmt.Errorf("creating certificate: %w", err)
	}

	if err := writePEM(certPath, "CERTIFICATE", der, 0644); err != nil {
		return err
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return fmt.Errorf("marshaling key: %w", err)
	}
	if err := writePEM(keyPath, "EC PRIVATE KEY", keyDER, 0600); err != nil {
		return err
	}
	return nil
}

func certDNSNames() []string {
	names := []string{"localhost"}
	if h, err := os.Hostname(); err == nil && h != "" && h != "localhost" {
		names = append(names, h)
	}
	return names
}

func certIPs() []net.IP {
	ips := []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() {
			continue
		}
		if v4 := ipnet.IP.To4(); v4 != nil {
			ips = append(ips, v4)
		}
	}
	return ips
}

func writePEM(path, blockType string, der []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: blockType, Bytes: der})
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
