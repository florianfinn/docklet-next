// Source: getarcaneapp/arcane/cli/pkg/generate/certs.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Moved into the hub command implementation; certificate builders use the non-command certgen package; M20 removal of inbound webhooks and remote CLI, retaining hub commands and typed refusals.

package generate

import (
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge/certgen"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"go.getarcane.app/acfs/atomic"
)

var (
	mtlsOutDir    string
	mtlsEnvID     string
	mtlsAppURL    string
	tlsOutDir     string
	tlsCommonName string
	tlsHosts      []string
	tlsCertName   string
	tlsKeyName    string
)

var mtlsCmd = &cobra.Command{
	Use:   "mtls",
	Short: "Generate Arcane edge mTLS assets",
	Long:  `Generate an Arcane-managed edge mTLS CA and agent client certificate bundle using ML-DSA-87.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return generateMTLSOutputInternal(cmd.OutOrStdout())
	},
}

var tlsCmd = &cobra.Command{
	Use:   "tls",
	Short: "Generate Arcane HTTPS TLS assets",
	Long:  `Generate a self-signed Arcane HTTPS server certificate bundle using ECDSA P-384.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return generateTLSOutputInternal(cmd.OutOrStdout())
	},
}

func init() {
	GenerateCmd.AddCommand(mtlsCmd)
	GenerateCmd.AddCommand(tlsCmd)

	mtlsCmd.Flags().StringVar(&mtlsOutDir, "out-dir", "./edge-mtls", "directory to write generated edge mTLS assets")
	mtlsCmd.Flags().StringVar(&mtlsEnvID, "env-id", "local-edge", "environment identifier to embed in the generated edge agent certificate")
	mtlsCmd.Flags().StringVar(&mtlsAppURL, "app-url", "http://localhost:3552", "Arcane manager app URL to embed as the edge mTLS SPIFFE trust domain")

	tlsCmd.Flags().StringVar(&tlsOutDir, "out-dir", "./arcane-tls", "directory to write generated Arcane TLS assets")
	tlsCmd.Flags().StringVar(&tlsCommonName, "common-name", "localhost", "certificate common name")
	tlsCmd.Flags().StringSliceVar(&tlsHosts, "host", []string{"localhost", "127.0.0.1"}, "DNS name or IP SAN to include; repeat flag to add more")
	tlsCmd.Flags().StringVar(&tlsCertName, "cert-name", "server.crt", "certificate file name to write under out-dir")
	tlsCmd.Flags().StringVar(&tlsKeyName, "key-name", "server.key", "private key file name to write under out-dir")
}

func generateMTLSOutputInternal(w io.Writer) error {
	outDir, err := filepath.Abs(strings.TrimSpace(mtlsOutDir))
	if err != nil {
		return fmt.Errorf("failed to resolve output directory: %w", err)
	}

	paths, err := generateEdgeMTLSBundleInternal(outDir, strings.TrimSpace(mtlsEnvID), strings.TrimSpace(mtlsAppURL))
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(w, "Generated Arcane edge mTLS assets (ML-DSA-87)")
	_, _ = fmt.Fprintf(w, "CA cert: %s\n", paths.CACertPath)
	_, _ = fmt.Fprintf(w, "CA key: %s\n", paths.CAKeyPath)
	_, _ = fmt.Fprintf(w, "Agent cert: %s\n", paths.ClientCertPath)
	_, _ = fmt.Fprintf(w, "Agent key: %s\n", paths.ClientKeyPath)
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Manager env")
	_, _ = fmt.Fprintf(w, "EDGE_MTLS_MODE=required\nEDGE_MTLS_CA_FILE=%s\n", paths.CACertPath)
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Agent env")
	_, _ = fmt.Fprintf(w, "EDGE_MTLS_MODE=required\nEDGE_MTLS_CA_FILE=%s\nEDGE_MTLS_CERT_FILE=%s\nEDGE_MTLS_KEY_FILE=%s\n", paths.CACertPath, paths.ClientCertPath, paths.ClientKeyPath)
	return nil
}

func generateTLSOutputInternal(w io.Writer) error {
	outDir, err := filepath.Abs(strings.TrimSpace(tlsOutDir))
	if err != nil {
		return fmt.Errorf("failed to resolve output directory: %w", err)
	}

	paths, err := generateServerTLSBundleInternal(outDir, strings.TrimSpace(tlsCommonName), tlsHosts, strings.TrimSpace(tlsCertName), strings.TrimSpace(tlsKeyName))
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(w, "Generated Arcane TLS assets (ECDSA P-384)")
	_, _ = fmt.Fprintf(w, "Server cert: %s\n", paths.CertPath)
	_, _ = fmt.Fprintf(w, "Server key: %s\n", paths.KeyPath)
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Manager env")
	_, _ = fmt.Fprintf(w, "TLS_ENABLED=true\nTLS_CERT_FILE=%s\nTLS_KEY_FILE=%s\n", paths.CertPath, paths.KeyPath)
	return nil
}

type edgeMTLSPaths struct {
	CACertPath     string
	CAKeyPath      string
	ClientCertPath string
	ClientKeyPath  string
}

type serverTLSPaths struct {
	CertPath string
	KeyPath  string
}

func generateEdgeMTLSBundleInternal(outDir, envID, appURL string) (*edgeMTLSPaths, error) {
	if envID == "" {
		return nil, errors.New("env ID is required")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	caKey, err := certgen.GenerateMLDSA87PrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate CA private key: %w", err)
	}
	caTemplate, err := certgen.NewEdgeMTLSCATemplate()
	if err != nil {
		return nil, err
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caKey.PublicKey(), caKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create CA certificate: %w", err)
	}

	clientKey, err := certgen.GenerateMLDSA87PrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate client private key: %w", err)
	}
	trustDomain := certgen.EdgeMTLSTrustDomain(appURL)
	dnsSANs := []string{"arcane-agent"}
	if trustDomain != "" {
		dnsSANs = append(dnsSANs, "arcane-agent."+trustDomain)
	}
	clientTemplate, err := certgen.NewEdgeMTLSClientTemplate("arcane-edge-"+envID, certgen.BuildEdgeMTLSURISAN(appURL, envID), dnsSANs)
	if err != nil {
		return nil, err
	}

	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, fmt.Errorf("failed to parse generated CA certificate: %w", err)
	}
	clientDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caCert, clientKey.PublicKey(), caKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create client certificate: %w", err)
	}

	paths := &edgeMTLSPaths{
		CACertPath:     filepath.Join(outDir, "ca.crt"),
		CAKeyPath:      filepath.Join(outDir, "ca.key"),
		ClientCertPath: filepath.Join(outDir, "agent.crt"),
		ClientKeyPath:  filepath.Join(outDir, "agent.key"),
	}
	if writeCertificateBundleErr := writeCertificateBundleInternal(paths.CACertPath, paths.CAKeyPath, caDER, caKey); writeCertificateBundleErr != nil {
		return nil, writeCertificateBundleErr
	}
	if writeClientBundleErr := writeCertificateBundleInternal(paths.ClientCertPath, paths.ClientKeyPath, clientDER, clientKey); writeClientBundleErr != nil {
		return nil, writeClientBundleErr
	}

	return paths, nil
}

func generateServerTLSBundleInternal(outDir, commonName string, hosts []string, certName, keyName string) (*serverTLSPaths, error) {
	if commonName == "" {
		return nil, errors.New("common name is required")
	}
	certName = filepath.Base(strings.TrimSpace(certName))
	if certName == "." || certName == "" {
		return nil, errors.New("certificate file name is required")
	}
	keyName = filepath.Base(strings.TrimSpace(keyName))
	if keyName == "." || keyName == "" {
		return nil, errors.New("key file name is required")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	privateKey, err := certgen.GenerateP384PrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate server private key: %w", err)
	}

	template, err := certgen.NewServerTLSTemplate(commonName, hosts)
	if err != nil {
		return nil, err
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create server certificate: %w", err)
	}

	paths := &serverTLSPaths{
		CertPath: filepath.Join(outDir, certName),
		KeyPath:  filepath.Join(outDir, keyName),
	}
	if writeCertificateBundleErr := writeCertificateBundleInternal(paths.CertPath, paths.KeyPath, certDER, privateKey); writeCertificateBundleErr != nil {
		return nil, writeCertificateBundleErr
	}
	return paths, nil
}

func writeCertificateBundleInternal(certPath, keyPath string, certDER []byte, privateKey crypto.PrivateKey) error {
	keyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %w", err)
	}
	if writePEMFileErr := writePEMFileInternal(certPath, "CERTIFICATE", certDER, 0o644); writePEMFileErr != nil {
		return writePEMFileErr
	}
	if writePrivateKeyErr := writePEMFileInternal(keyPath, "PRIVATE KEY", keyDER, 0o600); writePrivateKeyErr != nil {
		return writePrivateKeyErr
	}
	return nil
}

func writePEMFileInternal(path, blockType string, bytes []byte, perm os.FileMode) error {
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: bytes})
	if pemBytes == nil {
		return fmt.Errorf("failed to encode PEM file %s", path)
	}
	return atomic.WriteFile(path, pemBytes, perm)
}
