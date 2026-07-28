package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"gorm.io/gorm"
)

const shortLivedCertificateMaxHours = 168

func (s *CertificateService) AutoRenew(ctx context.Context, now time.Time) (int, error) {
	config, err := s.GetConfig()
	if err != nil {
		return 0, err
	}
	location, err := s.settingService.GetTimeLocation()
	if err != nil {
		return 0, err
	}
	localNow := now.In(location)
	lastRegularCheck, err := s.certificateCheckTimestamp(acmeLastRegularCheckAtKey)
	if err != nil {
		return 0, err
	}
	lastShortCheck, err := s.certificateCheckTimestamp(acmeLastShortCheckAtKey)
	if err != nil {
		return 0, err
	}

	regularCheckDue := regularCertificateCheckDue(
		localNow,
		time.Unix(lastRegularCheck, 0).In(location),
		config.CheckTime,
	)
	shortCheckDue := shortCertificateCheckDue(
		localNow,
		time.Unix(lastShortCheck, 0).In(location),
		config.ShortCheckTimesPerDay,
	)
	if !regularCheckDue && !shortCheckDue {
		return 0, nil
	}

	certificates, err := s.List()
	if err != nil {
		return 0, err
	}
	if err := markCertificateChecks(localNow, regularCheckDue, shortCheckDue); err != nil {
		return 0, err
	}

	renewed := 0
	var renewalErrors []error
	for _, certificate := range certificates {
		shortLived := isShortLivedCertificate(certificate)
		if (shortLived && !shortCheckDue) || (!shortLived && !regularCheckDue) {
			continue
		}
		renewBefore := time.Duration(config.RenewBeforeDays) * 24 * time.Hour
		if shortLived {
			renewBefore = time.Duration(config.ShortRenewBeforeHours) * time.Hour
		}
		if certificate.ExpiresAt.After(now.Add(renewBefore)) {
			continue
		}
		attemptedAt := now.UTC()
		if err := database.GetDB().Model(certificate).
			Update("last_renewal_attempt_at", attemptedAt).Error; err != nil {
			renewalErrors = append(renewalErrors, fmt.Errorf("certificate %d: %w", certificate.Id, err))
			continue
		}
		if err := s.renew(ctx, certificate, config.GlobalPrivateKey); err != nil {
			renewalErrors = append(renewalErrors, fmt.Errorf("certificate %d: %w", certificate.Id, err))
			continue
		}
		renewed++
	}
	return renewed, errors.Join(renewalErrors...)
}

func (s *CertificateService) certificateCheckTimestamp(key string) (int64, error) {
	value, err := s.settingService.getString(key)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(value, 10, 64)
}

func regularCertificateCheckDue(now, last time.Time, checkTime string) bool {
	if last.After(now) || sameLocalDate(now, last) {
		return false
	}
	parsed, err := time.Parse("15:04:05", checkTime)
	if err != nil {
		return false
	}
	scheduled := time.Date(
		now.Year(),
		now.Month(),
		now.Day(),
		parsed.Hour(),
		parsed.Minute(),
		parsed.Second(),
		0,
		now.Location(),
	)
	return !now.Before(scheduled)
}

func shortCertificateCheckDue(now, last time.Time, checksPerDay int) bool {
	if checksPerDay <= 0 || last.After(now) {
		return false
	}
	if last.Unix() <= 0 {
		return true
	}
	return now.Sub(last) >= 24*time.Hour/time.Duration(checksPerDay)
}

func sameLocalDate(first, second time.Time) bool {
	return first.Year() == second.Year() &&
		first.Month() == second.Month() &&
		first.Day() == second.Day()
}

func isShortLivedCertificate(certificate *model.Certificate) bool {
	validityHours := certificate.ValidityHours
	if validityHours <= 0 && !certificate.IssuedAt.IsZero() && !certificate.ExpiresAt.IsZero() {
		validityHours = certificateValidityHours(certificate.IssuedAt, certificate.ExpiresAt)
	}
	return validityHours > 0 && validityHours <= shortLivedCertificateMaxHours
}

func markCertificateChecks(now time.Time, regular, short bool) error {
	values := make(map[string]string, 2)
	if regular {
		values[acmeLastRegularCheckAtKey] = strconv.FormatInt(now.Unix(), 10)
	}
	if short {
		values[acmeLastShortCheckAtKey] = strconv.FormatInt(now.Unix(), 10)
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
