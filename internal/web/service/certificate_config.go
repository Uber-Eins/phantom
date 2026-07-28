package service

import (
	"errors"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/acmecert"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"gorm.io/gorm"
)

var certificateConfigMu sync.Mutex

const (
	acmeRenewBeforeDaysKey       = "acmeRenewBeforeDays"
	acmeShortRenewBeforeHoursKey = "acmeShortRenewBeforeHours"
	acmeShortCheckTimesPerDayKey = "acmeShortCheckTimesPerDay"
	acmeCheckTimeKey             = "acmeCheckTime"
	acmeDefaultEmailKey          = "acmeDefaultEmail"
	acmeGlobalPrivateKeyKey      = "acmeGlobalPrivateKey"
	acmeLastRegularCheckAtKey    = "acmeLastRegularCheckAt"
	acmeLastShortCheckAtKey      = "acmeLastShortCheckAt"
	defaultACMERenewBeforeDays   = 30
	defaultACMEShortRenewHours   = 24
	defaultACMEShortChecksPerDay = 4
	defaultACMECheckTime         = "05:00:00"
)

type CertificateConfig struct {
	RenewBeforeDays       int    `json:"renewBeforeDays"`
	ShortRenewBeforeHours int    `json:"shortRenewBeforeHours"`
	ShortCheckTimesPerDay int    `json:"shortCheckTimesPerDay"`
	CheckTime             string `json:"checkTime"`
	DefaultEmail          string `json:"defaultEmail"`
	GlobalPrivateKey      string `json:"globalPrivateKey"`
}

func (s *CertificateService) GetConfig() (*CertificateConfig, error) {
	certificateConfigMu.Lock()
	defer certificateConfigMu.Unlock()

	config := &CertificateConfig{}
	var err error
	if config.RenewBeforeDays, err = s.settingService.getInt(acmeRenewBeforeDaysKey); err != nil {
		return nil, err
	}
	if config.ShortRenewBeforeHours, err = s.settingService.getInt(acmeShortRenewBeforeHoursKey); err != nil {
		return nil, err
	}
	if config.ShortCheckTimesPerDay, err = s.settingService.getInt(acmeShortCheckTimesPerDayKey); err != nil {
		return nil, err
	}
	if config.CheckTime, err = s.settingService.getString(acmeCheckTimeKey); err != nil {
		return nil, err
	}
	if config.DefaultEmail, err = s.settingService.getString(acmeDefaultEmailKey); err != nil {
		return nil, err
	}
	if config.GlobalPrivateKey, err = s.settingService.getString(acmeGlobalPrivateKeyKey); err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.GlobalPrivateKey) == "" {
		config.GlobalPrivateKey, err = acmecert.GenerateAccountPrivateKeyPEM()
		if err != nil {
			return nil, err
		}
		if err := s.settingService.setString(acmeGlobalPrivateKeyKey, config.GlobalPrivateKey); err != nil {
			return nil, err
		}
	}
	if err := validateCertificateConfig(config); err != nil {
		return nil, err
	}
	return config, nil
}

func (s *CertificateService) SaveConfig(config *CertificateConfig) error {
	certificateConfigMu.Lock()
	defer certificateConfigMu.Unlock()

	if config == nil {
		return fmt.Errorf("ACME configuration is required")
	}
	config.DefaultEmail = strings.TrimSpace(config.DefaultEmail)
	config.GlobalPrivateKey = strings.TrimSpace(config.GlobalPrivateKey)
	if config.GlobalPrivateKey == "" {
		generated, err := acmecert.GenerateAccountPrivateKeyPEM()
		if err != nil {
			return err
		}
		config.GlobalPrivateKey = generated
	}
	if err := validateCertificateConfig(config); err != nil {
		return err
	}

	values := map[string]string{
		acmeRenewBeforeDaysKey:       strconv.Itoa(config.RenewBeforeDays),
		acmeShortRenewBeforeHoursKey: strconv.Itoa(config.ShortRenewBeforeHours),
		acmeShortCheckTimesPerDayKey: strconv.Itoa(config.ShortCheckTimesPerDay),
		acmeCheckTimeKey:             config.CheckTime,
		acmeDefaultEmailKey:          config.DefaultEmail,
		acmeGlobalPrivateKeyKey:      config.GlobalPrivateKey,
	}
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		for key, value := range values {
			if err := upsertCertificateSetting(tx, key, value); err != nil {
				return err
			}
		}
		return nil
	})
}

func validateCertificateConfig(config *CertificateConfig) error {
	if config.RenewBeforeDays < 1 || config.RenewBeforeDays > 90 {
		return fmt.Errorf("certificate renewal days must be between 1 and 90")
	}
	if config.ShortRenewBeforeHours < 24 || config.ShortRenewBeforeHours > 168 {
		return fmt.Errorf("short-lived certificate renewal hours must be between 24 and 168")
	}
	if config.ShortCheckTimesPerDay < 4 || config.ShortCheckTimesPerDay > 6 {
		return fmt.Errorf("short-lived certificate checks per day must be between 4 and 6")
	}
	if _, err := time.Parse("15:04:05", config.CheckTime); err != nil {
		return fmt.Errorf("ACME check time must use HH:mm:ss: %w", err)
	}
	if config.DefaultEmail != "" {
		address, err := mail.ParseAddress(config.DefaultEmail)
		if err != nil || address.Address != config.DefaultEmail {
			return fmt.Errorf("ACME default email is invalid")
		}
	}
	if err := acmecert.ValidateAccountPrivateKeyPEM(config.GlobalPrivateKey); err != nil {
		return fmt.Errorf("ACME global private key is invalid: %w", err)
	}
	return nil
}

func upsertCertificateSetting(tx *gorm.DB, key, value string) error {
	var setting model.Setting
	result := tx.Where("key = ?", key).First(&setting)
	switch {
	case errors.Is(result.Error, gorm.ErrRecordNotFound):
		return tx.Create(&model.Setting{Key: key, Value: value}).Error
	case result.Error != nil:
		return result.Error
	default:
		return tx.Model(&setting).Update("value", value).Error
	}
}
