// Source: getarcaneapp/arcane/cli/pkg/generate/certs.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Certificate templates and key builders moved into a non-command package; M20 removal of inbound webhooks and remote CLI, retaining hub commands and typed refusals.

package certgen

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/mldsa"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"strings"
	"time"
)

const certValidity = 5 * 365 * 24 * time.Hour

// GenerateP384PrivateKey generates an ECDSA P-384 private key.
func GenerateP384PrivateKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
}

// GenerateMLDSA87PrivateKey generates an ML-DSA-87 private key.
func GenerateMLDSA87PrivateKey() (*mldsa.PrivateKey, error) {
	return mldsa.GenerateKey(mldsa.MLDSA87())
}

// NewEdgeMTLSCATemplate builds Arcane's edge mTLS CA certificate template.
func NewEdgeMTLSCATemplate() (*x509.Certificate, error) {
	template, err := newCertificateTemplateInternal("Arcane Edge mTLS CA")
	if err != nil {
		return nil, err
	}
	template.Subject = pkix.Name{CommonName: "Arcane Edge mTLS CA", Organization: []string{"Arcane"}}
	template.IsCA = true
	template.BasicConstraintsValid = true
	template.MaxPathLen = 1
	template.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature
	return template, nil
}

// NewEdgeMTLSClientTemplate builds an edge-agent client certificate template.
func NewEdgeMTLSClientTemplate(commonName string, uriSAN *url.URL, dnsSANs []string) (*x509.Certificate, error) {
	commonName = strings.TrimSpace(commonName)
	if commonName == "" {
		return nil, errors.New("common name is required")
	}
	template, err := newCertificateTemplateInternal(commonName)
	if err != nil {
		return nil, err
	}
	template.NotBefore = time.Now().UTC().Add(-24 * time.Hour)
	template.Subject = pkix.Name{
		CommonName:   commonName,
		Organization: []string{"Arcane Edge Agents"},
	}
	template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	template.KeyUsage = x509.KeyUsageDigitalSignature
	if uriSAN != nil {
		template.URIs = []*url.URL{uriSAN}
	}
	for _, dnsSAN := range dnsSANs {
		dnsSAN = strings.TrimSpace(dnsSAN)
		if dnsSAN != "" {
			template.DNSNames = append(template.DNSNames, dnsSAN)
		}
	}
	return template, nil
}

// NewServerTLSTemplate builds a self-signed Arcane HTTPS server certificate template.
func NewServerTLSTemplate(commonName string, hosts []string) (*x509.Certificate, error) {
	commonName = strings.TrimSpace(commonName)
	if commonName == "" {
		return nil, errors.New("common name is required")
	}
	template, err := newCertificateTemplateInternal(commonName)
	if err != nil {
		return nil, err
	}
	template.Subject = pkix.Name{CommonName: commonName, Organization: []string{"Arcane"}}
	template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	template.KeyUsage = x509.KeyUsageDigitalSignature

	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
			continue
		}
		template.DNSNames = append(template.DNSNames, host)
	}
	if len(template.DNSNames) == 0 && len(template.IPAddresses) == 0 {
		template.DNSNames = []string{commonName}
	}
	return template, nil
}

// EdgeMTLSTrustDomain returns the SPIFFE trust domain derived from an Arcane app URL.
func EdgeMTLSTrustDomain(appURL string) string {
	appURL = strings.TrimSpace(appURL)
	if appURL == "" {
		return ""
	}
	parseURL := appURL
	if !strings.Contains(parseURL, "://") {
		parseURL = "https://" + parseURL
	}
	parsed, err := url.Parse(parseURL)
	if err == nil {
		if host := strings.TrimSpace(parsed.Hostname()); host != "" {
			return strings.ToLower(strings.TrimSuffix(host, "."))
		}
	}
	return strings.ToLower(strings.TrimSuffix(appURL, "."))
}

// BuildEdgeMTLSURISAN returns the SPIFFE URI SAN for an edge environment ID.
func BuildEdgeMTLSURISAN(appURL, envID string) *url.URL {
	trustDomain := EdgeMTLSTrustDomain(appURL)
	envID = strings.TrimSpace(envID)
	if trustDomain == "" || envID == "" {
		return nil
	}
	return &url.URL{Scheme: "spiffe", Host: trustDomain, Path: "/edge/" + envID}
}

func newCertificateTemplateInternal(commonName string) (*x509.Certificate, error) {
	serial, err := randomSerialInternal()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	return &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(certValidity),
	}, nil
}

func randomSerialInternal() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to generate certificate serial: %w", err)
	}
	return serial, nil
}
