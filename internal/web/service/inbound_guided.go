package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/nginxfront"

	"gorm.io/gorm"
)

func (s *InboundService) AddGuidedInbound(
	inbound *model.Inbound,
	fronting *model.InboundFronting,
) (*model.Inbound, bool, error) {
	if inbound == nil || fronting == nil {
		return inbound, false, fmt.Errorf("guided inbound request is incomplete")
	}
	fronting.Template = strings.ToLower(strings.TrimSpace(fronting.Template))
	fronting.DecoyMode = strings.ToLower(strings.TrimSpace(fronting.DecoyMode))
	fronting.DecoyValue = strings.TrimSpace(fronting.DecoyValue)
	if err := s.ensureGuidedPublicPortAvailable(); err != nil {
		return inbound, false, err
	}
	templateName, ok := nginxfront.TemplateName(fronting.Template)
	if !ok {
		return inbound, false, fmt.Errorf("unsupported guided template %q", fronting.Template)
	}
	socket, err := nginxfront.NewSocketPath(fronting.Template)
	if err != nil {
		return inbound, false, err
	}
	inbound.Listen = socket
	inbound.Port = 0
	inbound.Tag = templateName
	desiredEnable := inbound.Enable
	inbound.Enable = false
	inbound.Fronting = nil

	created, _, err := s.AddInbound(inbound)
	if err != nil {
		return inbound, false, err
	}
	fronting.InboundId = created.Id
	if err := nginxfront.ValidateCandidate(created, fronting, 0); err != nil {
		_, _ = s.DelInbound(created.Id)
		return created, false, err
	}

	db := database.GetDB()
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(fronting).Error; err != nil {
			return err
		}
		return tx.Model(&model.Inbound{}).
			Where("id = ?", created.Id).
			Update("enable", desiredEnable).Error
	}); err != nil {
		_, _ = s.DelInbound(created.Id)
		return created, false, err
	}
	created.Enable = desiredEnable
	created.Fronting = fronting

	if err := nginxfront.Reconcile(); err != nil {
		s.rollbackGuidedInbound(created.Id)
		return created, false, err
	}
	if !desiredEnable {
		return created, false, nil
	}

	rt, available := localRuntime()
	if !available {
		return created, true, nil
	}
	runtimeInbound, buildErr := s.buildRuntimeInboundForAPI(db, created)
	if buildErr != nil {
		logger.Warning("guided inbound runtime projection failed:", buildErr)
		return created, true, nil
	}
	if err := rt.AddInbound(context.Background(), runtimeInbound); err != nil {
		logger.Warning("guided inbound runtime add failed:", err)
		return created, true, nil
	}
	return created, false, nil
}

func (s *InboundService) ensureGuidedPublicPortAvailable() error {
	var rows []*model.Inbound
	if err := database.GetDB().Where("port = ?", nginxfront.PublicPort).Find(&rows).Error; err != nil {
		return err
	}
	for _, inbound := range rows {
		if nginxfront.IsManagedSocket(inbound.Listen) {
			continue
		}
		if inboundTransports(inbound.Protocol, inbound.StreamSettings, inbound.Settings)&transportTCP != 0 {
			return fmt.Errorf("TCP port %d is already used by inbound %q", nginxfront.PublicPort, inbound.Remark)
		}
	}
	return nil
}

func (s *InboundService) rollbackGuidedInbound(id int) {
	db := database.GetDB()
	var inbound model.Inbound
	_ = db.First(&inbound, id).Error
	_ = db.Model(&model.Inbound{}).Where("id = ?", id).Update("enable", false).Error
	_ = db.Where("inbound_id = ?", id).Delete(&model.InboundFronting{}).Error
	_, _ = s.DelInbound(id)
	_ = nginxfront.Reconcile()
	nginxfront.RemoveInboundAssets(id, inbound.Listen)
}
