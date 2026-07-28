package acmecert

import (
	"crypto"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

func (m *Manager) loadOrCreateAccountKey(ca, email string) (crypto.Signer, bool, error) {
	hash := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	accountDir := filepath.Join(m.baseDir, ".accounts", ca, hex.EncodeToString(hash[:8]))
	keyPath := filepath.Join(accountDir, "account.key")

	if data, err := os.ReadFile(keyPath); err == nil {
		key, parseErr := parsePrivateKey(data)
		return key, false, parseErr
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, false, err
	}

	if err := os.MkdirAll(accountDir, 0o750); err != nil {
		return nil, false, fmt.Errorf("create ACME account directory: %w", err)
	}
	key, err := generateAccountKey()
	if err != nil {
		return nil, false, err
	}
	encoded, err := marshalPrivateKey(key)
	if err != nil {
		return nil, false, err
	}
	file, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		data, readErr := os.ReadFile(keyPath)
		if readErr != nil {
			return nil, false, readErr
		}
		existing, parseErr := parsePrivateKey(data)
		return existing, false, parseErr
	}
	if err != nil {
		return nil, false, fmt.Errorf("create ACME account key: %w", err)
	}
	if _, err = file.Write(encoded); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		_ = os.Remove(keyPath)
		return nil, false, fmt.Errorf("write ACME account key: %w", err)
	}
	if closeErr != nil {
		_ = os.Remove(keyPath)
		return nil, false, fmt.Errorf("close ACME account key: %w", closeErr)
	}
	return key, true, nil
}

func (m *Manager) saveCertificate(
	remark string,
	certificatePEM []byte,
	privateKeyPEM []byte,
) (certificateFile string, keyFile string, err error) {
	if err := os.MkdirAll(m.baseDir, 0o750); err != nil {
		return "", "", fmt.Errorf("create ACME certificate directory: %w", err)
	}
	directory, err := m.createCertificateDirectory(remark)
	if err != nil {
		return "", "", err
	}

	certificateFile = filepath.Join(directory, "fullchain.pem")
	keyFile = filepath.Join(directory, "privkey.pem")
	if err := os.WriteFile(certificateFile, certificatePEM, 0o644); err != nil {
		_ = os.Remove(directory)
		return "", "", fmt.Errorf("write certificate file: %w", err)
	}
	if err := os.WriteFile(keyFile, privateKeyPEM, 0o600); err != nil {
		_ = os.Remove(certificateFile)
		_ = os.Remove(directory)
		return "", "", fmt.Errorf("write private key file: %w", err)
	}
	return certificateFile, keyFile, nil
}

func (m *Manager) loadManagedCertificateKey(
	certificateFile string,
	keyFile string,
) (crypto.Signer, error) {
	if err := m.validateManagedCertificateFiles(certificateFile, keyFile); err != nil {
		return nil, err
	}
	encoded, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("read certificate private key: %w", err)
	}
	key, err := parsePrivateKey(encoded)
	if err != nil {
		return nil, fmt.Errorf("parse certificate private key: %w", err)
	}
	return key, nil
}

func (m *Manager) saveRenewedCertificate(
	certificateFile string,
	keyFile string,
	certificatePEM []byte,
) error {
	if err := m.validateManagedCertificateFiles(certificateFile, keyFile); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(certificateFile), ".fullchain-*.tmp")
	if err != nil {
		return fmt.Errorf("create renewed certificate file: %w", err)
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)

	if err = file.Chmod(0o644); err == nil {
		_, err = file.Write(certificatePEM)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return fmt.Errorf("write renewed certificate file: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("close renewed certificate file: %w", closeErr)
	}
	if err := os.Rename(tempPath, certificateFile); err != nil {
		return fmt.Errorf("replace renewed certificate file: %w", err)
	}
	return nil
}

func (m *Manager) validateManagedCertificateFiles(certificateFile, keyFile string) error {
	base, err := filepath.Abs(m.baseDir)
	if err != nil {
		return err
	}
	certificatePath, err := filepath.Abs(certificateFile)
	if err != nil {
		return err
	}
	keyPath, err := filepath.Abs(keyFile)
	if err != nil {
		return err
	}
	for _, path := range []string{certificatePath, keyPath} {
		relative, relErr := filepath.Rel(base, path)
		if relErr != nil || relative == ".." ||
			strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("certificate file %q is outside the managed ACME directory", path)
		}
	}
	if filepath.Dir(certificatePath) != filepath.Dir(keyPath) {
		return fmt.Errorf("certificate and private key are not in the same managed directory")
	}
	return nil
}

func (m *Manager) createCertificateDirectory(remark string) (string, error) {
	slug := certificateSlug(remark)
	for suffix := 1; suffix <= 999; suffix++ {
		name := slug
		if suffix > 1 {
			name = fmt.Sprintf("%s-%d", slug, suffix)
		}
		candidate := filepath.Join(m.baseDir, name)
		err := os.Mkdir(candidate, 0o750)
		if err == nil {
			return candidate, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("create certificate directory: %w", err)
		}
	}
	return "", fmt.Errorf("too many certificates use the remark %q", remark)
}

func certificateSlug(remark string) string {
	var slug strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(remark)) {
		allowed := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_'
		if allowed {
			slug.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			slug.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(slug.String(), "-_")
	if result == "" {
		return "certificate"
	}
	return result
}
