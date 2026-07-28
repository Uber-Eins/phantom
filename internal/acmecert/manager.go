package acmecert

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Uber-Eins/phantom/v3/internal/logger"

	"golang.org/x/crypto/acme"
)

type issuedMaterial struct {
	remark         string
	identifiers    []string
	certificatePEM []byte
	privateKeyPEM  []byte
	leaf           *x509.Certificate
}

func (m *Manager) Issue(ctx context.Context, request IssueRequest) (*IssueResult, error) {
	material, err := m.issueMaterial(ctx, request, nil)
	if err != nil {
		return nil, err
	}
	certificateFile, keyFile, err := m.saveCertificate(
		material.remark,
		material.certificatePEM,
		material.privateKeyPEM,
	)
	if err != nil {
		return nil, err
	}
	return material.result(certificateFile, keyFile), nil
}

func (m *Manager) Renew(
	ctx context.Context,
	request IssueRequest,
	certificateFile string,
	keyFile string,
) (*IssueResult, error) {
	certificateKey, err := m.loadManagedCertificateKey(certificateFile, keyFile)
	if err != nil {
		return nil, err
	}
	if err := validateCertificateKeyType(certificateKey, request.KeyType); err != nil {
		return nil, err
	}
	material, err := m.issueMaterial(ctx, request, certificateKey)
	if err != nil {
		return nil, err
	}
	if err := m.saveRenewedCertificate(certificateFile, keyFile, material.certificatePEM); err != nil {
		return nil, err
	}
	return material.result(certificateFile, keyFile), nil
}

func (m *Manager) issueMaterial(
	ctx context.Context,
	request IssueRequest,
	certificateKey crypto.Signer,
) (*issuedMaterial, error) {
	identifiers, err := validateRequest(request)
	if err != nil {
		return nil, err
	}
	request.Remark = strings.TrimSpace(request.Remark)
	request.Email = strings.TrimSpace(request.Email)
	request.CloudflareToken = strings.TrimSpace(request.CloudflareToken)
	if strings.TrimSpace(m.baseDir) == "" {
		return nil, fmt.Errorf("ACME certificate directory is not configured")
	}

	accountKey := m.accountKey
	accountKeyCreated := false
	if accountKey == nil {
		accountKey, accountKeyCreated, err = m.loadOrCreateAccountKey(request.CA, request.Email)
		if err != nil {
			return nil, err
		}
	}
	client := &acme.Client{
		Key:          accountKey,
		DirectoryURL: directoryURL(request.CA),
		HTTPClient:   m.httpClient,
	}
	if err := m.registerAccount(ctx, client, request.CA, request.Email, accountKeyCreated); err != nil {
		return nil, err
	}

	if certificateKey == nil {
		certificateKey, err = generateCertificateKey(request.KeyType)
		if err != nil {
			return nil, err
		}
	}
	certificateDER, err := m.obtainCertificate(
		ctx,
		client,
		certificateKey,
		identifiers,
		request.CloudflareToken,
	)
	if err != nil {
		return nil, err
	}
	certificatePEM, leaf, err := encodeCertificateChain(certificateDER)
	if err != nil {
		return nil, err
	}
	keyPEM, err := marshalPrivateKey(certificateKey)
	if err != nil {
		return nil, err
	}
	if _, err := tls.X509KeyPair(certificatePEM, keyPEM); err != nil {
		return nil, fmt.Errorf("issued certificate and private key do not match: %w", err)
	}

	return &issuedMaterial{
		remark:         request.Remark,
		identifiers:    identifiers,
		certificatePEM: certificatePEM,
		privateKeyPEM:  keyPEM,
		leaf:           leaf,
	}, nil
}

func (m *issuedMaterial) result(certificateFile, keyFile string) *IssueResult {
	return &IssueResult{
		Remark:          m.remark,
		Identifiers:     m.identifiers,
		CertificateFile: certificateFile,
		KeyFile:         keyFile,
		IssuedAt:        m.leaf.NotBefore,
		ExpiresAt:       m.leaf.NotAfter,
	}
}

func (m *Manager) obtainCertificate(
	ctx context.Context,
	client *acme.Client,
	certificateKey crypto.Signer,
	identifiers []string,
	cloudflareToken string,
) ([][]byte, error) {
	order, err := client.AuthorizeOrder(ctx, acme.DomainIDs(identifiers...))
	if err != nil {
		return nil, fmt.Errorf("create ACME order: %w", err)
	}

	dnsClient := &cloudflareClient{
		baseURL:    m.cloudflareURL,
		token:      cloudflareToken,
		httpClient: m.httpClient,
	}
	for _, authorizationURL := range order.AuthzURLs {
		if err := m.completeDNSAuthorization(ctx, client, dnsClient, authorizationURL); err != nil {
			return nil, err
		}
	}

	readyOrder, err := client.WaitOrder(ctx, order.URI)
	if err != nil {
		return nil, fmt.Errorf("wait for ACME order: %w", err)
	}
	csr, err := createCSR(certificateKey, identifiers)
	if err != nil {
		return nil, err
	}
	certificates, _, err := client.CreateOrderCert(
		ctx,
		readyOrder.FinalizeURL,
		csr,
		true,
	)
	if err != nil {
		return nil, fmt.Errorf("finalize ACME order: %w", err)
	}
	return certificates, nil
}

func (m *Manager) completeDNSAuthorization(
	ctx context.Context,
	client *acme.Client,
	dnsClient *cloudflareClient,
	authorizationURL string,
) error {
	authorization, err := client.GetAuthorization(ctx, authorizationURL)
	if err != nil {
		return fmt.Errorf("get ACME authorization: %w", err)
	}
	if authorization.Status == acme.StatusValid {
		return nil
	}

	var challenge *acme.Challenge
	for _, candidate := range authorization.Challenges {
		if candidate.Type == "dns-01" {
			challenge = candidate
			break
		}
	}
	if challenge == nil {
		return fmt.Errorf("ACME server did not offer a DNS-01 challenge for %q", authorization.Identifier.Value)
	}

	value, err := client.DNS01ChallengeRecord(challenge.Token)
	if err != nil {
		return fmt.Errorf("build ACME DNS challenge: %w", err)
	}
	identifier := strings.TrimPrefix(authorization.Identifier.Value, "*.")
	recordName := "_acme-challenge." + identifier
	zoneID, recordID, err := dnsClient.createTXT(ctx, identifier, recordName, value)
	if err != nil {
		return fmt.Errorf("create Cloudflare DNS challenge for %q: %w", identifier, err)
	}

	cleanup := func() error {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		if err := dnsClient.deleteRecord(cleanupCtx, zoneID, recordID); err != nil {
			return fmt.Errorf("remove Cloudflare DNS challenge for %q: %w", identifier, err)
		}
		return nil
	}
	if err := m.waitForDNSRecord(ctx, recordName, value); err != nil {
		return errors.Join(err, cleanup())
	}
	if _, err := client.Accept(ctx, challenge); err != nil {
		return errors.Join(fmt.Errorf("accept ACME challenge for %q: %w", identifier, err), cleanup())
	}
	if _, err := client.WaitAuthorization(ctx, authorizationURL); err != nil {
		return errors.Join(fmt.Errorf("validate ACME challenge for %q: %w", identifier, err), cleanup())
	}
	if err := cleanup(); err != nil {
		logger.Warningf("ACME certificate: %v", err)
	}
	return nil
}

func createCSR(key crypto.Signer, identifiers []string) ([]byte, error) {
	template := &x509.CertificateRequest{DNSNames: identifiers}
	csr, err := x509.CreateCertificateRequest(rand.Reader, template, key)
	if err != nil {
		return nil, fmt.Errorf("create certificate signing request: %w", err)
	}
	return csr, nil
}

func encodeCertificateChain(chain [][]byte) ([]byte, *x509.Certificate, error) {
	if len(chain) == 0 {
		return nil, nil, fmt.Errorf("ACME server returned an empty certificate chain")
	}
	leaf, err := x509.ParseCertificate(chain[0])
	if err != nil {
		return nil, nil, fmt.Errorf("parse issued certificate: %w", err)
	}

	var encoded bytes.Buffer
	for _, certificate := range chain {
		if err := pem.Encode(&encoded, &pem.Block{Type: "CERTIFICATE", Bytes: certificate}); err != nil {
			return nil, nil, fmt.Errorf("encode issued certificate: %w", err)
		}
	}
	return encoded.Bytes(), leaf, nil
}

func directoryURL(ca string) string {
	if ca == CAZeroSSL {
		return zeroSSLDirectoryURL
	}
	return letsEncryptDirectoryURL
}
