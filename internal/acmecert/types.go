package acmecert

import (
	"context"
	"crypto"
	"net/http"
	"time"
)

const (
	AddMethodACME = "acme"

	CAZeroSSL            = "zerossl"
	CALetsEncrypt        = "letsencrypt"
	ValidationCloudflare = "cloudflare"

	KeyEC256   = "EC256"
	KeyEC384   = "EC384"
	KeyRSA2048 = "RSA2048"
	KeyRSA4096 = "RSA4096"

	CertificateDomain = "domain"
	CertificateIP     = "ip"
)

const (
	letsEncryptDirectoryURL = "https://acme-v02.api.letsencrypt.org/directory"
	zeroSSLDirectoryURL     = "https://acme.zerossl.com/v2/DV90"
	defaultCloudflareURL    = "https://api.cloudflare.com/client/v4"
	defaultZeroSSLEABURL    = "https://api.zerossl.com/acme/eab-credentials-email"
)

type IssueRequest struct {
	Remark           string
	AddMethod        string
	CA               string
	ValidationMethod string
	CloudflareToken  string
	Identifiers      string
	Email            string
	KeyType          string
	CertificateType  string
}

type IssueResult struct {
	Remark          string    `json:"remark"`
	Identifiers     []string  `json:"identifiers"`
	CertificateFile string    `json:"certificateFile"`
	KeyFile         string    `json:"keyFile"`
	IssuedAt        time.Time `json:"issuedAt"`
	ExpiresAt       time.Time `json:"expiresAt"`
}

type Manager struct {
	httpClient       *http.Client
	baseDir          string
	accountKey       crypto.Signer
	cloudflareURL    string
	zeroSSLEABURL    string
	waitForDNSRecord func(ctx context.Context, name, value string) error
}

func NewManager(httpClient *http.Client, baseDir string) *Manager {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 45 * time.Second}
	}
	return &Manager{
		httpClient:       httpClient,
		baseDir:          baseDir,
		cloudflareURL:    defaultCloudflareURL,
		zeroSSLEABURL:    defaultZeroSSLEABURL,
		waitForDNSRecord: waitForTXTRecord,
	}
}

// SetAccountPrivateKeyPEM configures one account key shared by all ACME issuers.
func (m *Manager) SetAccountPrivateKeyPEM(encoded string) error {
	key, err := parsePrivateKey([]byte(encoded))
	if err != nil {
		return err
	}
	m.accountKey = key
	return nil
}
