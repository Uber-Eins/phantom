package service

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Uber-Eins/phantom/v3/internal/acmecert"
	"github.com/Uber-Eins/phantom/v3/internal/config"
	"github.com/Uber-Eins/phantom/v3/internal/database"
	"github.com/Uber-Eins/phantom/v3/internal/database/model"
)

type certificateIssuer interface {
	Issue(context.Context, acmecert.IssueRequest) (*acmecert.IssueResult, error)
	Renew(context.Context, acmecert.IssueRequest, string, string) (*acmecert.IssueResult, error)
}

type certificateIssuerFactory func(
	httpClient *http.Client,
	baseDir string,
	accountPrivateKey string,
) (certificateIssuer, error)

type CertificateService struct {
	settingService SettingService
	issuerFactory  certificateIssuerFactory
}

func (s *CertificateService) List() ([]*model.Certificate, error) {
	var certificates []*model.Certificate
	if err := database.GetDB().Order("id asc").Find(&certificates).Error; err != nil {
		return nil, err
	}
	for _, certificate := range certificates {
		populateCertificateIdentifiers(certificate)
	}
	return certificates, nil
}

func (s *CertificateService) Issue(
	ctx context.Context,
	request acmecert.IssueRequest,
) (*model.Certificate, error) {
	config, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	issuer, err := s.newIssuer(config.GlobalPrivateKey)
	if err != nil {
		return nil, err
	}
	result, err := issuer.Issue(ctx, request)
	if err != nil {
		return nil, err
	}

	certificate := certificateFromIssue(request, result)
	if err := database.GetDB().Create(certificate).Error; err != nil {
		cleanupIssuedFiles(result)
		return nil, fmt.Errorf("save issued certificate: %w", err)
	}
	populateCertificateIdentifiers(certificate)
	return certificate, nil
}

func (s *CertificateService) Update(
	id int,
	request acmecert.IssueRequest,
) (*model.Certificate, error) {
	var certificate model.Certificate
	if err := database.GetDB().First(&certificate, id).Error; err != nil {
		return nil, err
	}

	updates := map[string]any{
		"remark":            strings.TrimSpace(request.Remark),
		"add_method":        request.AddMethod,
		"ca":                request.CA,
		"validation_method": request.ValidationMethod,
		"identifiers":       strings.TrimSpace(request.Identifiers),
		"email":             strings.TrimSpace(request.Email),
		"key_type":          request.KeyType,
		"certificate_type":  request.CertificateType,
	}
	if token := strings.TrimSpace(request.CloudflareToken); token != "" {
		updates["cloudflare_token"] = token
	}
	if err := database.GetDB().Model(&certificate).Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := database.GetDB().First(&certificate, id).Error; err != nil {
		return nil, err
	}
	populateCertificateIdentifiers(&certificate)
	return &certificate, nil
}

func (s *CertificateService) Delete(id int) error {
	var certificate model.Certificate
	if err := database.GetDB().First(&certificate, id).Error; err != nil {
		return err
	}
	if err := database.GetDB().Delete(&certificate).Error; err != nil {
		return err
	}
	cleanupIssuedFiles(&acmecert.IssueResult{
		CertificateFile: certificate.CertificateFile,
		KeyFile:         certificate.KeyFile,
	})
	return nil
}

func (s *CertificateService) renew(
	ctx context.Context,
	certificate *model.Certificate,
	globalPrivateKey string,
) error {
	issuer, err := s.newIssuer(globalPrivateKey)
	if err != nil {
		return err
	}
	result, err := issuer.Renew(
		ctx,
		requestFromCertificate(certificate),
		certificate.CertificateFile,
		certificate.KeyFile,
	)
	if err != nil {
		return err
	}
	validityHours := certificateValidityHours(result.IssuedAt, result.ExpiresAt)
	return database.GetDB().Model(certificate).Updates(map[string]any{
		"issued_at":      result.IssuedAt,
		"expires_at":     result.ExpiresAt,
		"validity_hours": validityHours,
	}).Error
}

func (s *CertificateService) newIssuer(globalPrivateKey string) (certificateIssuer, error) {
	httpClient := s.settingService.NewProxiedHTTPClient(45 * time.Second)
	baseDir := filepath.Join(config.GetDBFolderPath(), "certificates", "acme")
	if s.issuerFactory != nil {
		return s.issuerFactory(httpClient, baseDir, globalPrivateKey)
	}
	manager := acmecert.NewManager(httpClient, baseDir)
	if err := manager.SetAccountPrivateKeyPEM(globalPrivateKey); err != nil {
		return nil, fmt.Errorf("configure ACME global private key: %w", err)
	}
	return manager, nil
}

func certificateFromIssue(
	request acmecert.IssueRequest,
	result *acmecert.IssueResult,
) *model.Certificate {
	return &model.Certificate{
		Remark:           result.Remark,
		AddMethod:        request.AddMethod,
		CA:               request.CA,
		ValidationMethod: request.ValidationMethod,
		CloudflareToken:  strings.TrimSpace(request.CloudflareToken),
		Identifiers:      strings.Join(result.Identifiers, "\n"),
		Email:            strings.TrimSpace(request.Email),
		KeyType:          request.KeyType,
		CertificateType:  request.CertificateType,
		CertificateFile:  result.CertificateFile,
		KeyFile:          result.KeyFile,
		IssuedAt:         result.IssuedAt,
		ExpiresAt:        result.ExpiresAt,
		ValidityHours:    certificateValidityHours(result.IssuedAt, result.ExpiresAt),
	}
}

func requestFromCertificate(certificate *model.Certificate) acmecert.IssueRequest {
	return acmecert.IssueRequest{
		Remark:           certificate.Remark,
		AddMethod:        certificate.AddMethod,
		CA:               certificate.CA,
		ValidationMethod: certificate.ValidationMethod,
		CloudflareToken:  certificate.CloudflareToken,
		Identifiers:      certificate.Identifiers,
		Email:            certificate.Email,
		KeyType:          certificate.KeyType,
		CertificateType:  certificate.CertificateType,
	}
}

func certificateValidityHours(issuedAt, expiresAt time.Time) int {
	hours := int(expiresAt.Sub(issuedAt).Hours())
	if hours < 0 {
		return 0
	}
	return hours
}

func cleanupIssuedFiles(result *acmecert.IssueResult) {
	if result == nil {
		return
	}
	_ = os.Remove(result.CertificateFile)
	_ = os.Remove(result.KeyFile)
	if filepath.Dir(result.CertificateFile) == filepath.Dir(result.KeyFile) {
		_ = os.Remove(filepath.Dir(result.CertificateFile))
	}
}
