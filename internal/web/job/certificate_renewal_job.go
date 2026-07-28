package job

import (
	"context"
	"time"

	"github.com/Uber-Eins/phantom/v3/internal/logger"
	"github.com/Uber-Eins/phantom/v3/internal/web/service"
)

type CertificateRenewalJob struct {
	certificateService *service.CertificateService
}

func NewCertificateRenewalJob() *CertificateRenewalJob {
	return &CertificateRenewalJob{certificateService: &service.CertificateService{}}
}

func (j *CertificateRenewalJob) Run() {
	if j.certificateService == nil {
		j.certificateService = &service.CertificateService{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	renewed, err := j.certificateService.AutoRenew(ctx, time.Now())
	if err != nil {
		logger.Warning("ACME certificate auto-renewal error:", err)
	}
	if renewed > 0 {
		logger.Infof("Renewed %d ACME certificate(s)", renewed)
	}
}
