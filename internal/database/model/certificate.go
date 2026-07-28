package model

import "time"

// Certificate stores an ACME-managed certificate and the inputs required to renew it.
type Certificate struct {
	Id                   int       `json:"id" gorm:"primaryKey;autoIncrement"`
	Remark               string    `json:"remark"`
	AddMethod            string    `json:"addMethod"`
	CA                   string    `json:"ca"`
	ValidationMethod     string    `json:"validationMethod"`
	CloudflareToken      string    `json:"-" gorm:"type:text"`
	Identifiers          string    `json:"identifiers" gorm:"type:text"`
	Email                string    `json:"email"`
	KeyType              string    `json:"keyType"`
	CertificateType      string    `json:"certificateType"`
	CertificateFile      string    `json:"certificateFile"`
	KeyFile              string    `json:"keyFile"`
	IssuedAt             time.Time `json:"issuedAt"`
	ExpiresAt            time.Time `json:"expiresAt" gorm:"index"`
	ValidityHours        int       `json:"-" gorm:"default:0"`
	LastRenewalAttemptAt time.Time `json:"-" gorm:"index"`
	CreatedAt            time.Time `json:"-"`
	UpdatedAt            time.Time `json:"-"`
}
