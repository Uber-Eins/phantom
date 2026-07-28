package database

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Uber-Eins/phantom/v3/internal/database/model"

	"gorm.io/gorm"
)

// MigrateSingleMachine removes state that belonged to the former multi-node,
// public-subscription and external-notification features. It intentionally
// leaves historical columns on core tables in place: rebuilding SQLite tables
// just to remove unused columns would make upgrades needlessly risky.
//
// The migration is transactional and safe to run after every restore/startup.
func MigrateSingleMachine(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}

	return db.Transaction(func(tx *gorm.DB) error {
		remoteIDs, err := remoteInboundIDs(tx)
		if err != nil {
			return err
		}
		if err := preserveLocalClientTraffic(tx, remoteIDs); err != nil {
			return err
		}
		if err := deleteRemoteInboundRelations(tx, remoteIDs); err != nil {
			return err
		}
		if len(remoteIDs) > 0 {
			if err := tx.Where("id IN ?", remoteIDs).Delete(&model.Inbound{}).Error; err != nil {
				return err
			}
		}

		if err := clearLegacyCoreColumns(tx); err != nil {
			return err
		}
		if err := cleanInboundJSON(tx); err != nil {
			return err
		}
		if err := deleteRemovedSettings(tx); err != nil {
			return err
		}
		if err := dropRemovedTables(tx); err != nil {
			return err
		}
		return nil
	})
}

func remoteInboundIDs(tx *gorm.DB) ([]int, error) {
	if !tx.Migrator().HasTable("inbounds") ||
		!tx.Migrator().HasColumn(&model.Inbound{}, "node_id") {
		return nil, nil
	}
	var ids []int
	err := tx.Table("inbounds").Where("node_id IS NOT NULL").Pluck("id", &ids).Error
	return ids, err
}

// A traffic row can be owned by a remote inbound even when the same client is
// also attached locally. Move those counters to a local attachment before the
// remote inbound is deleted; only genuinely remote-only traffic is discarded.
func preserveLocalClientTraffic(tx *gorm.DB, remoteIDs []int) error {
	if len(remoteIDs) == 0 ||
		!tx.Migrator().HasTable("client_traffics") ||
		!tx.Migrator().HasTable("clients") ||
		!tx.Migrator().HasTable("client_inbounds") {
		return nil
	}

	var rows []struct {
		ID    int
		Email string
	}
	if err := tx.Table("client_traffics").
		Select("id, email").
		Where("inbound_id IN ?", remoteIDs).
		Scan(&rows).Error; err != nil {
		return err
	}

	for _, row := range rows {
		var localInboundID int
		err := tx.Table("client_inbounds AS ci").
			Select("ci.inbound_id").
			Joins("JOIN clients AS c ON c.id = ci.client_id").
			Joins("JOIN inbounds AS i ON i.id = ci.inbound_id").
			Where("c.email = ? AND i.node_id IS NULL", row.Email).
			Order("ci.inbound_id ASC").
			Limit(1).
			Scan(&localInboundID).Error
		if err != nil {
			return err
		}
		if localInboundID > 0 {
			if err := tx.Table("client_traffics").
				Where("id = ?", row.ID).
				Update("inbound_id", localInboundID).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func deleteRemoteInboundRelations(tx *gorm.DB, remoteIDs []int) error {
	if len(remoteIDs) == 0 {
		return nil
	}
	deletes := []struct {
		table string
		where string
	}{
		{"client_inbounds", "inbound_id IN ?"},
		{"client_traffics", "inbound_id IN ?"},
		{"inbound_frontings", "inbound_id IN ?"},
		{"inbound_fallbacks", "master_id IN ? OR child_id IN ?"},
		{"hosts", "inbound_id IN ?"},
	}
	for _, item := range deletes {
		if !tx.Migrator().HasTable(item.table) {
			continue
		}
		var err error
		if item.table == "inbound_fallbacks" {
			err = tx.Exec(
				"DELETE FROM inbound_fallbacks WHERE master_id IN ? OR child_id IN ?",
				remoteIDs,
				remoteIDs,
			).Error
		} else {
			err = tx.Exec("DELETE FROM "+item.table+" WHERE "+item.where, remoteIDs).Error
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func clearLegacyCoreColumns(tx *gorm.DB) error {
	if tx.Migrator().HasTable("clients") {
		updates := map[string]any{}
		for column, value := range map[string]any{
			"group_name": "",
			"tg_id":      int64(0),
			"limit_ip":   0,
		} {
			if tx.Migrator().HasColumn(&model.ClientRecord{}, column) {
				updates[column] = value
			}
		}
		if len(updates) > 0 {
			if err := tx.Table("clients").Where("1 = 1").Updates(updates).Error; err != nil {
				return err
			}
		}
	}

	if tx.Migrator().HasTable("inbounds") {
		updates := map[string]any{}
		for column, value := range map[string]any{
			"node_id":             nil,
			"sub_sort_index":      0,
			"share_addr_strategy": "",
			"share_addr":          "",
			"origin_node_guid":    "",
		} {
			if tx.Migrator().HasColumn(&model.Inbound{}, column) {
				updates[column] = value
			}
		}
		if len(updates) > 0 {
			if err := tx.Table("inbounds").Where("1 = 1").Updates(updates).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func cleanInboundJSON(tx *gorm.DB) error {
	if !tx.Migrator().HasTable("inbounds") {
		return nil
	}
	var rows []struct {
		ID             int
		Settings       string
		StreamSettings string
	}
	if err := tx.Table("inbounds").
		Select("id, settings, stream_settings").
		Find(&rows).Error; err != nil {
		return err
	}

	for _, row := range rows {
		updates := map[string]any{}
		if cleaned, changed := cleanSettingsClients(row.Settings); changed {
			updates["settings"] = cleaned
		}
		if cleaned, changed := removeExternalProxy(row.StreamSettings); changed {
			updates["stream_settings"] = cleaned
		}
		if len(updates) > 0 {
			if err := tx.Table("inbounds").Where("id = ?", row.ID).Updates(updates).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func cleanSettingsClients(raw string) (string, bool) {
	if strings.TrimSpace(raw) == "" {
		return raw, false
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return raw, false
	}
	clients, ok := settings["clients"].([]any)
	if !ok {
		return raw, false
	}
	changed := false
	for _, entry := range clients {
		client, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		for _, key := range []string{"group", "tgId", "limitIp"} {
			if _, exists := client[key]; exists {
				delete(client, key)
				changed = true
			}
		}
	}
	if !changed {
		return raw, false
	}
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return raw, false
	}
	return string(out), true
}

func removeExternalProxy(raw string) (string, bool) {
	if strings.TrimSpace(raw) == "" {
		return raw, false
	}
	var stream map[string]any
	if err := json.Unmarshal([]byte(raw), &stream); err != nil {
		return raw, false
	}
	if _, exists := stream["externalProxy"]; !exists {
		return raw, false
	}
	delete(stream, "externalProxy")
	out, err := json.MarshalIndent(stream, "", "  ")
	if err != nil {
		return raw, false
	}
	return string(out), true
}

func deleteRemovedSettings(tx *gorm.DB) error {
	if !tx.Migrator().HasTable("settings") {
		return nil
	}
	prefixes := []string{"tg", "smtp", "ldap", "sub"}
	query := tx.Model(&model.Setting{}).Select("id")
	for i, prefix := range prefixes {
		condition := "key LIKE ?"
		value := prefix + "%"
		if i == 0 {
			query = query.Where(condition, value)
		} else {
			query = query.Or(condition, value)
		}
	}
	query = query.Or("key IN ?", []string{
		"apiToken",
		"nodeMtlsCaCertPem",
		"nodeMtlsCaKeyPem",
		"nodeMtlsClientCertPem",
		"nodeMtlsClientKeyPem",
		"nodeMtlsClientCAPem",
		"externalTrafficInformEnable",
		"externalTrafficInformURI",
		"remarkTemplate",
		"outboundDownThreshold",
		"panelGuid",
	})
	var ids []int
	if err := query.Pluck("id", &ids).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	return tx.Where("id IN ?", ids).Delete(&model.Setting{}).Error
}

func dropRemovedTables(tx *gorm.DB) error {
	tables := []string{
		"nodes",
		"hosts",
		"client_groups",
		"api_tokens",
		"client_external_links",
		"node_client_traffics",
		"node_client_ips",
		"client_global_traffics",
		"inbound_client_ips",
	}
	for _, table := range tables {
		if !tx.Migrator().HasTable(table) {
			continue
		}
		if err := tx.Migrator().DropTable(table); err != nil {
			return fmt.Errorf("drop %s: %w", table, err)
		}
	}
	return nil
}
