package nginxfront

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"gorm.io/gorm"
)

func Get(inboundID int) (*model.InboundFronting, error) {
	var row model.InboundFronting
	err := database.GetDB().Where("inbound_id = ?", inboundID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func Annotate(db *gorm.DB, inbounds []*model.Inbound) {
	if len(inbounds) == 0 {
		return
	}
	ids := make([]int, 0, len(inbounds))
	for _, inbound := range inbounds {
		ids = append(ids, inbound.Id)
	}
	var rows []model.InboundFronting
	if err := db.Where("inbound_id IN ?", ids).Find(&rows).Error; err != nil {
		return
	}
	byID := make(map[int]*model.InboundFronting, len(rows))
	for i := range rows {
		row := rows[i]
		byID[row.InboundId] = &row
	}
	for _, inbound := range inbounds {
		inbound.Fronting = byID[inbound.Id]
	}
}

func loadRoutes(db *gorm.DB, enabledOnly bool) ([]route, error) {
	var frontings []model.InboundFronting
	if err := db.Order("inbound_id ASC").Find(&frontings).Error; err != nil {
		return nil, err
	}
	if len(frontings) == 0 {
		return nil, nil
	}
	ids := make([]int, 0, len(frontings))
	for _, fronting := range frontings {
		ids = append(ids, fronting.InboundId)
	}
	var inbounds []model.Inbound
	query := db.Where("id IN ?", ids)
	if enabledOnly {
		query = query.Where("enable = ?", true)
	}
	if err := query.Order("id ASC").Find(&inbounds).Error; err != nil {
		return nil, err
	}
	inboundByID := make(map[int]*model.Inbound, len(inbounds))
	for i := range inbounds {
		inboundByID[inbounds[i].Id] = &inbounds[i]
	}
	routes := make([]route, 0, len(frontings))
	for i := range frontings {
		inbound := inboundByID[frontings[i].InboundId]
		if inbound == nil {
			continue
		}
		parsed, err := parseRoute(inbound, &frontings[i])
		if err != nil {
			return nil, err
		}
		routes = append(routes, parsed)
	}
	return routes, nil
}

func ValidateCandidate(inbound *model.Inbound, fronting *model.InboundFronting, ignoreID int) error {
	candidate, err := parseRoute(inbound, fronting)
	if err != nil {
		return err
	}
	routes, err := loadRoutes(database.GetDB(), false)
	if err != nil {
		return err
	}
	filtered := routes[:0]
	for _, current := range routes {
		if current.Inbound.Id != ignoreID {
			filtered = append(filtered, current)
		}
	}
	return validateTopology(append(filtered, candidate))
}

func PublicPortClaimed() (bool, error) {
	var count int64
	err := database.GetDB().Model(&model.InboundFronting{}).Count(&count).Error
	return count > 0, err
}

func RemoveInboundAssets(inboundID int, listen string) {
	for _, path := range []string{
		filepath.Join(ConfigDir(), "certs", fmt.Sprintf("inbound-%d.crt", inboundID)),
		filepath.Join(ConfigDir(), "certs", fmt.Sprintf("inbound-%d.key", inboundID)),
	} {
		_ = os.Remove(path)
	}
	if IsManagedSocket(listen) {
		_ = os.Remove(socketPath(listen))
	}
}
