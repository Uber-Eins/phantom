package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/Uber-Eins/phantom/v3/internal/database"
	"github.com/Uber-Eins/phantom/v3/internal/database/model"
	"github.com/Uber-Eins/phantom/v3/internal/web/runtime"
	"github.com/Uber-Eins/phantom/v3/internal/xray"
	"gorm.io/gorm"
)

const onlineGracePeriodMs int64 = 20000

func localRuntime() (runtime.Runtime, bool) {
	manager := runtime.GetManager()
	if manager == nil {
		return nil, false
	}
	return manager.Runtime(), true
}

func (s *InboundService) runtimeFor(_ *model.Inbound) (runtime.Runtime, error) {
	rt, ok := localRuntime()
	if !ok {
		return nil, fmt.Errorf("local runtime not initialised")
	}
	return rt, nil
}

func (s *InboundService) GetOnlineClients() []string {
	process := currentXrayProcess()
	if process == nil {
		return []string{}
	}
	return process.GetOnlineClients()
}

func (s *InboundService) GetClientsLastOnline() (map[string]int64, error) {
	var rows []xray.ClientTraffic
	err := database.GetDB().
		Model(&xray.ClientTraffic{}).
		Select("email, last_online").
		Find(&rows).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	result := make(map[string]int64, len(rows))
	for _, row := range rows {
		result[row.Email] = row.LastOnline
	}
	return result, nil
}

func (s *InboundService) RefreshLocalOnlineClients(activeEmails, activeInboundTags []string) {
	if process := currentXrayProcess(); process != nil {
		process.RefreshLocalOnline(activeEmails, activeInboundTags, time.Now().UnixMilli(), onlineGracePeriodMs)
	}
}
