package database

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/crypto"
	"github.com/mhsanaei/3x-ui/v3/internal/util/random"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var db *gorm.DB

const (
	defaultUsername = "admin"
	defaultPassword = "admin"
)

func allModels() []any {
	return []any{
		&model.User{},
		&model.Inbound{},
		&model.OutboundTraffics{},
		&model.Setting{},
		&xray.ClientTraffic{},
		&model.HistoryOfSeeders{},
		&model.ClientRecord{},
		&model.ClientInbound{},
		&model.InboundFallback{},
		&model.InboundFronting{},
		&model.OutboundSubscription{},
		&model.Certificate{},
	}
}

func initModels() error {
	models := allModels()
	for _, mdl := range models {
		if err := db.AutoMigrate(mdl); err != nil {
			if isIgnorableDuplicateColumnErr(db, err, mdl) {
				log.Printf("Ignoring duplicate column during auto migration for %T: %v", mdl, err)
				continue
			}
			log.Printf("Error auto migrating model: %v", err)
			return err
		}
	}
	if err := dropLegacyInboundPortUnique(); err != nil {
		return err
	}
	if err := pruneOrphanedClientInbounds(); err != nil {
		return err
	}
	if err := pruneOrphanedInboundFrontings(); err != nil {
		return err
	}
	if err := repairOverflowedTrafficCounters(); err != nil {
		return err
	}
	if err := dedupeInboundSettingsClients(); err != nil {
		return err
	}
	if err := migrateLegacySocksInboundsToMixed(); err != nil {
		return err
	}
	if err := migrateShadowsocksRemovedCiphers(); err != nil {
		return err
	}
	if err := migrateVmessRemovedSecurities(); err != nil {
		return err
	}
	return nil
}

func pruneOrphanedInboundFrontings() error {
	return db.Exec(`
		DELETE FROM inbound_frontings
		WHERE inbound_id NOT IN (SELECT id FROM inbounds)
	`).Error
}

type sqliteIndexListRow struct {
	Name   string `gorm:"column:name"`
	Unique int    `gorm:"column:unique"`
	Origin string `gorm:"column:origin"`
}

func sqliteUniquePortIndexes() (autoIndexes, explicitIndexes []string, err error) {
	var list []sqliteIndexListRow
	if err = db.Raw(`PRAGMA index_list('inbounds')`).Scan(&list).Error; err != nil {
		return nil, nil, err
	}
	for _, idx := range list {
		if idx.Unique != 1 {
			continue
		}
		var cols []struct {
			Name string `gorm:"column:name"`
		}
		if err = db.Raw(`PRAGMA index_info("` + idx.Name + `")`).Scan(&cols).Error; err != nil {
			return nil, nil, err
		}
		if len(cols) != 1 || cols[0].Name != "port" {
			continue
		}
		if idx.Origin == "c" {
			explicitIndexes = append(explicitIndexes, idx.Name)
		} else {
			autoIndexes = append(autoIndexes, idx.Name)
		}
	}
	return autoIndexes, explicitIndexes, nil
}

// dropLegacyInboundPortUnique removes the old UNIQUE on inbounds.port, which
// AutoMigrate never drops. TCP and UDP inbounds may legitimately share a port.
func dropLegacyInboundPortUnique() error {
	autoIndexes, explicitIndexes, err := sqliteUniquePortIndexes()
	if err != nil {
		return err
	}
	for _, name := range explicitIndexes {
		if err := db.Exec(`DROP INDEX IF EXISTS "` + name + `"`).Error; err != nil {
			return err
		}
	}
	if len(autoIndexes) == 0 {
		return nil
	}
	log.Printf("Rebuilding inbounds table to drop the legacy UNIQUE constraint on port")
	return rebuildInboundsWithoutInlineUniquePort()
}

func sqliteTableColumns(tx *gorm.DB, table string) ([]string, error) {
	var rows []struct {
		Name string `gorm:"column:name"`
	}
	if err := tx.Raw(`PRAGMA table_info("` + table + `")`).Scan(&rows).Error; err != nil {
		return nil, err
	}
	cols := make([]string, 0, len(rows))
	for _, r := range rows {
		cols = append(cols, r.Name)
	}
	return cols, nil
}

func rebuildInboundsWithoutInlineUniquePort() error {
	return db.Transaction(func(tx *gorm.DB) error {
		var list []sqliteIndexListRow
		if err := tx.Raw(`PRAGMA index_list('inbounds')`).Scan(&list).Error; err != nil {
			return err
		}
		for _, idx := range list {
			if idx.Origin != "c" {
				continue
			}
			if err := tx.Exec(`DROP INDEX IF EXISTS "` + idx.Name + `"`).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec(`ALTER TABLE inbounds RENAME TO inbounds_legacy_rebuild`).Error; err != nil {
			return err
		}
		if err := tx.Migrator().CreateTable(&model.Inbound{}); err != nil {
			return err
		}
		newCols, err := sqliteTableColumns(tx, "inbounds")
		if err != nil {
			return err
		}
		oldCols, err := sqliteTableColumns(tx, "inbounds_legacy_rebuild")
		if err != nil {
			return err
		}
		oldSet := make(map[string]struct{}, len(oldCols))
		for _, c := range oldCols {
			oldSet[c] = struct{}{}
		}
		shared := make([]string, 0, len(newCols))
		for _, c := range newCols {
			if _, ok := oldSet[c]; ok {
				shared = append(shared, `"`+c+`"`)
			}
		}
		colList := strings.Join(shared, ", ")
		if err := tx.Exec(`INSERT INTO inbounds (` + colList + `) SELECT ` + colList + ` FROM inbounds_legacy_rebuild`).Error; err != nil {
			return err
		}
		return tx.Exec(`DROP TABLE inbounds_legacy_rebuild`).Error
	})
}

func seedWireguardPeersToClients() error {
	var history []string
	if err := db.Model(&model.HistoryOfSeeders{}).Pluck("seeder_name", &history).Error; err != nil {
		return err
	}
	if slices.Contains(history, "WireguardPeersToClients") {
		return nil
	}

	var inbounds []model.Inbound
	if err := db.Where("protocol = ?", string(model.WireGuard)).Find(&inbounds).Error; err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		usedEmails := map[string]struct{}{}
		var existingEmails []string
		if err := tx.Model(&model.ClientRecord{}).Pluck("email", &existingEmails).Error; err != nil {
			return err
		}
		for _, e := range existingEmails {
			usedEmails[e] = struct{}{}
		}

		for _, inbound := range inbounds {
			if strings.TrimSpace(inbound.Settings) == "" {
				continue
			}
			var settings map[string]any
			if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
				log.Printf("WireguardPeersToClients: skip inbound %d (invalid settings json): %v", inbound.Id, err)
				continue
			}
			peers, ok := settings["peers"].([]any)
			if !ok || len(peers) == 0 {
				continue
			}

			var linkCount int64
			if err := tx.Model(&model.ClientInbound{}).Where("inbound_id = ?", inbound.Id).Count(&linkCount).Error; err != nil {
				return err
			}
			if linkCount > 0 {
				continue
			}

			clientObjs := make([]any, 0, len(peers))
			for i, raw := range peers {
				obj, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				email := wireguardPeerEmail(inbound.Remark, obj, i, usedEmails)
				usedEmails[email] = struct{}{}
				obj["email"] = email
				if sub, _ := obj["subId"].(string); strings.TrimSpace(sub) == "" {
					obj["subId"] = random.NumLower(16)
				}
				if _, ok := obj["enable"]; !ok {
					obj["enable"] = true
				}

				blob, err := json.Marshal(obj)
				if err != nil {
					continue
				}
				var c model.Client
				if err := json.Unmarshal(blob, &c); err != nil {
					log.Printf("WireguardPeersToClients: skip peer in inbound %d: %v", inbound.Id, err)
					continue
				}
				c.Email = email

				incoming := c.ToRecord()
				var row model.ClientRecord
				err = tx.Where("email = ?", email).First(&row).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					if err := tx.Create(incoming).Error; err != nil {
						return err
					}
					row = *incoming
				} else if err != nil {
					return err
				} else {
					model.MergeClientRecord(&row, incoming)
					if err := tx.Save(&row).Error; err != nil {
						return err
					}
				}

				link := model.ClientInbound{ClientId: row.Id, InboundId: inbound.Id}
				if err := tx.Where("client_id = ? AND inbound_id = ?", row.Id, inbound.Id).
					FirstOrCreate(&link).Error; err != nil {
					return err
				}

				clientObjs = append(clientObjs, obj)
			}

			delete(settings, "peers")
			settings["clients"] = clientObjs
			newSettings, err := json.Marshal(settings)
			if err != nil {
				return err
			}
			if err := tx.Model(&model.Inbound{}).Where("id = ?", inbound.Id).
				Update("settings", string(newSettings)).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "WireguardPeersToClients"}).Error
	})
}

func wireguardPeerEmail(remark string, peer map[string]any, index int, used map[string]struct{}) string {
	base := strings.TrimSpace(remark)
	if base == "" {
		base = "wg"
	}
	suffix := strconv.Itoa(index + 1)
	if c, ok := peer["comment"].(string); ok && strings.TrimSpace(c) != "" {
		suffix = strings.TrimSpace(c)
	}
	email := strings.ReplaceAll(base+"-"+suffix, " ", "-")
	candidate := email
	for n := 2; ; n++ {
		if _, taken := used[candidate]; !taken {
			return candidate
		}
		candidate = email + "-" + strconv.Itoa(n)
	}
}

// seedMtprotoSecretsToClients converts each legacy single-secret mtproto inbound
// into a one-client inbound so MTProto joins the shared multi-client model: the
// inbound-level secret becomes the first client's FakeTLS secret, and a
// ClientRecord + client_inbounds link are created so per-client traffic, limits,
// and share links work exactly like every other protocol. One-time, self-gated
// on the "MtprotoSecretsToClients" seeder row. Mirrors seedWireguardPeersToClients.
func seedMtprotoSecretsToClients() error {
	var history []string
	if err := db.Model(&model.HistoryOfSeeders{}).Pluck("seeder_name", &history).Error; err != nil {
		return err
	}
	if slices.Contains(history, "MtprotoSecretsToClients") {
		return nil
	}

	var inbounds []model.Inbound
	if err := db.Where("protocol = ?", string(model.MTProto)).Find(&inbounds).Error; err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		usedEmails := map[string]struct{}{}
		var existingEmails []string
		if err := tx.Model(&model.ClientRecord{}).Pluck("email", &existingEmails).Error; err != nil {
			return err
		}
		for _, e := range existingEmails {
			usedEmails[e] = struct{}{}
		}

		for _, inbound := range inbounds {
			if strings.TrimSpace(inbound.Settings) == "" {
				continue
			}
			var settings map[string]any
			if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
				log.Printf("MtprotoSecretsToClients: skip inbound %d (invalid settings json): %v", inbound.Id, err)
				continue
			}
			if clients, ok := settings["clients"].([]any); ok && len(clients) > 0 {
				continue
			}

			var linkCount int64
			if err := tx.Model(&model.ClientInbound{}).Where("inbound_id = ?", inbound.Id).Count(&linkCount).Error; err != nil {
				return err
			}
			if linkCount > 0 {
				continue
			}

			secret, _ := settings["secret"].(string)
			secret = strings.TrimSpace(secret)
			if secret == "" {
				domain, _ := settings["fakeTlsDomain"].(string)
				secret = model.GenerateFakeTLSSecret(strings.TrimSpace(domain))
			}

			email := mtprotoInboundClientEmail(inbound.Remark, usedEmails)
			usedEmails[email] = struct{}{}

			obj := map[string]any{
				"email":  email,
				"secret": secret,
				"enable": true,
				"subId":  random.NumLower(16),
			}
			c := model.Client{Email: email, Secret: secret, Enable: true, SubID: obj["subId"].(string)}

			incoming := c.ToRecord()
			var row model.ClientRecord
			err := tx.Where("email = ?", email).First(&row).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := tx.Create(incoming).Error; err != nil {
					return err
				}
				row = *incoming
			} else if err != nil {
				return err
			} else {
				model.MergeClientRecord(&row, incoming)
				if err := tx.Save(&row).Error; err != nil {
					return err
				}
			}

			link := model.ClientInbound{ClientId: row.Id, InboundId: inbound.Id}
			if err := tx.Where("client_id = ? AND inbound_id = ?", row.Id, inbound.Id).
				FirstOrCreate(&link).Error; err != nil {
				return err
			}

			delete(settings, "secret")
			settings["clients"] = []any{obj}
			newSettings, err := json.Marshal(settings)
			if err != nil {
				return err
			}
			if err := tx.Model(&model.Inbound{}).Where("id = ?", inbound.Id).
				Update("settings", string(newSettings)).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "MtprotoSecretsToClients"}).Error
	})
}

// stripMtprotoInboundSecrets removes the vestigial inbound-level `secret` from
// every mtproto inbound. seedMtprotoSecretsToClients already drops it while
// converting legacy single-secret inbounds, but inbounds that already had clients
// kept the dead field, and the old HealMtprotoSecret regenerated it on every
// save. mtg and every share link read only per-client secrets, so the
// inbound-level value is dead data that once leaked into stale, unusable links.
// One-time, self-gated on the "StripMtprotoInboundSecrets" seeder row.
func stripMtprotoInboundSecrets() error {
	var history []string
	if err := db.Model(&model.HistoryOfSeeders{}).Pluck("seeder_name", &history).Error; err != nil {
		return err
	}
	if slices.Contains(history, "StripMtprotoInboundSecrets") {
		return nil
	}

	var inbounds []model.Inbound
	if err := db.Where("protocol = ?", string(model.MTProto)).Find(&inbounds).Error; err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for _, inbound := range inbounds {
			stripped, ok := model.StripMtprotoInboundSecret(inbound.Settings)
			if !ok {
				continue
			}
			if err := tx.Model(&model.Inbound{}).Where("id = ?", inbound.Id).
				Update("settings", stripped).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "StripMtprotoInboundSecrets"}).Error
	})
}

// mtprotoInboundClientEmail derives a stable, unique client email for a migrated
// mtproto inbound from its remark.
func mtprotoInboundClientEmail(remark string, used map[string]struct{}) string {
	base := strings.TrimSpace(remark)
	if base == "" {
		base = "mtproto"
	}
	email := strings.ReplaceAll(base, " ", "-")
	candidate := email
	for n := 2; ; n++ {
		if _, taken := used[candidate]; !taken {
			return candidate
		}
		candidate = email + "-" + strconv.Itoa(n)
	}
}

func pruneOrphanedClientInbounds() error {
	res := db.Exec("DELETE FROM client_inbounds WHERE inbound_id NOT IN (SELECT id FROM inbounds)")
	if res.Error != nil {
		log.Printf("Error pruning orphaned client_inbounds rows: %v", res.Error)
		return res.Error
	}
	if res.RowsAffected > 0 {
		log.Printf("Pruned %d orphaned client_inbounds row(s)", res.RowsAffected)
	}
	return nil
}

// migrateLegacySocksInboundsToMixed renames legacy socks inbounds to mixed.
// The protocol enum dropped socks in favor of mixed (identical settings shape,
// same behavior plus HTTP on the shared port), so rows predating the rename
// fail model validation and prevent the local Xray config from loading.
func migrateLegacySocksInboundsToMixed() error {
	res := db.Exec("UPDATE inbounds SET protocol = 'mixed' WHERE protocol = 'socks'")
	if res.Error != nil {
		log.Printf("Error migrating legacy socks inbounds to mixed: %v", res.Error)
		return res.Error
	}
	if res.RowsAffected > 0 {
		log.Printf("Migrated %d legacy socks inbound(s) to mixed", res.RowsAffected)
	}
	return nil
}

// migrateShadowsocksRemovedCiphers rewrites shadowsocks inbounds still using
// the "none"/"plain" ciphers that xray-core v26.7.11 removed; one such row
// makes the whole generated config unbuildable and keeps xray from starting.
func migrateShadowsocksRemovedCiphers() error {
	var inbounds []model.Inbound
	if err := db.Where("protocol = ?", model.Shadowsocks).Find(&inbounds).Error; err != nil {
		return err
	}
	migrated := int64(0)
	for _, inbound := range inbounds {
		if strings.TrimSpace(inbound.Settings) == "" {
			continue
		}
		var settings map[string]any
		if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
			continue
		}
		changed := false
		if method, _ := settings["method"].(string); method != "" {
			if replacement, removed := model.ReplaceRemovedShadowsocksCipher(method); removed {
				settings["method"] = replacement
				changed = true
			}
		}
		if clients, ok := settings["clients"].([]any); ok {
			for i := range clients {
				cm, ok := clients[i].(map[string]any)
				if !ok {
					continue
				}
				method, _ := cm["method"].(string)
				if replacement, removed := model.ReplaceRemovedShadowsocksCipher(method); removed {
					cm["method"] = replacement
					clients[i] = cm
					changed = true
				}
			}
		}
		if !changed {
			continue
		}
		newSettings, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			log.Printf("migrateShadowsocksRemovedCiphers: skip inbound %d (marshal failed): %v", inbound.Id, err)
			continue
		}
		if err := db.Model(&model.Inbound{}).Where("id = ?", inbound.Id).
			Update("settings", string(newSettings)).Error; err != nil {
			return err
		}
		migrated++
	}
	if migrated > 0 {
		log.Printf("Rewrote removed shadowsocks cipher on %d inbound(s)", migrated)
	}
	return nil
}

// migrateVmessRemovedSecurities rewrites the vmess "none"/"zero" security
// values that xray-core v26.7.11 removed to "auto" (what the core now treats
// them as), on both the clients column and each vmess inbound's settings.
func migrateVmessRemovedSecurities() error {
	res := db.Exec("UPDATE clients SET security = 'auto' WHERE security IN ('none', 'zero')")
	if res.Error != nil {
		log.Printf("Error migrating removed vmess security values on clients: %v", res.Error)
		return res.Error
	}
	if res.RowsAffected > 0 {
		log.Printf("Migrated %d client(s) off removed vmess security values", res.RowsAffected)
	}
	var inbounds []model.Inbound
	if err := db.Where("protocol = ?", model.VMESS).Find(&inbounds).Error; err != nil {
		return err
	}
	migrated := int64(0)
	for _, inbound := range inbounds {
		if strings.TrimSpace(inbound.Settings) == "" {
			continue
		}
		var settings map[string]any
		if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
			continue
		}
		clients, ok := settings["clients"].([]any)
		if !ok {
			continue
		}
		changed := false
		for i := range clients {
			cm, ok := clients[i].(map[string]any)
			if !ok {
				continue
			}
			if security, _ := cm["security"].(string); security == "none" || security == "zero" {
				cm["security"] = "auto"
				clients[i] = cm
				changed = true
			}
		}
		if !changed {
			continue
		}
		newSettings, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			log.Printf("migrateVmessRemovedSecurities: skip inbound %d (marshal failed): %v", inbound.Id, err)
			continue
		}
		if err := db.Model(&model.Inbound{}).Where("id = ?", inbound.Id).
			Update("settings", string(newSettings)).Error; err != nil {
			return err
		}
		migrated++
	}
	if migrated > 0 {
		log.Printf("Rewrote removed vmess security values on %d inbound(s)", migrated)
	}
	return nil
}

// repairOverflowedTrafficCounters heals traffic counters that historic
// compounding bugs pushed past int64: on SQLite an overflowing INTEGER is
// silently promoted to REAL, after which the column no longer scans into the
// Go int64 field and every reader of the table fails (#5762). REAL cells are
// cast back to INTEGER (SQLite caps the cast at math.MaxInt64), then values
// are clamped into [0, TrafficMax] so the next delta cannot overflow again.
func repairOverflowedTrafficCounters() error {
	targets := []struct {
		table   string
		columns []string
	}{
		{"client_traffics", []string{"up", "down"}},
		{"inbounds", []string{"up", "down"}},
		{"outbound_traffics", []string{"up", "down", "total"}},
	}
	for _, target := range targets {
		for _, col := range target.columns {
			statements := []string{
				fmt.Sprintf("UPDATE %s SET %s = CAST(%s AS INTEGER) WHERE typeof(%s) = 'real'", target.table, col, col, col),
				fmt.Sprintf("UPDATE %s SET %s = %d WHERE %s > %d", target.table, col, TrafficMax, col, TrafficMax),
				fmt.Sprintf("UPDATE %s SET %s = 0 WHERE %s < 0", target.table, col, col),
			}
			var repaired int64
			for _, statement := range statements {
				res := db.Exec(statement)
				if res.Error != nil {
					log.Printf("Error repairing %s.%s: %v", target.table, col, res.Error)
					return res.Error
				}
				repaired += res.RowsAffected
			}
			if repaired > 0 {
				log.Printf("Repaired %d overflowed %s.%s value(s)", repaired, target.table, col)
			}
		}
	}
	return nil
}

// dedupeInboundSettingsClients collapses duplicate same-email entries inside
// every inbound's settings.clients array, keeping the first occurrence.
// Retried client adds on older builds could append the same client several
// times (#5770), which the client lists then rendered as phantom duplicates.
// It is idempotent and writes only changed rows.
func dedupeInboundSettingsClients() error {
	var inbounds []model.Inbound
	if err := db.Find(&inbounds).Error; err != nil {
		return err
	}
	repaired := int64(0)
	for _, inbound := range inbounds {
		if strings.TrimSpace(inbound.Settings) == "" {
			continue
		}
		var settings map[string]any
		if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
			continue
		}
		clients, _ := settings["clients"].([]any)
		if len(clients) < 2 {
			continue
		}
		seen := make(map[string]struct{}, len(clients))
		kept := make([]any, 0, len(clients))
		for _, c := range clients {
			if cm, ok := c.(map[string]any); ok {
				if email, _ := cm["email"].(string); email != "" {
					key := strings.ToLower(email)
					if _, dup := seen[key]; dup {
						continue
					}
					seen[key] = struct{}{}
				}
			}
			kept = append(kept, c)
		}
		if len(kept) == len(clients) {
			continue
		}
		settings["clients"] = kept
		newSettings, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			log.Printf("dedupeInboundSettingsClients: skip inbound %d (marshal failed): %v", inbound.Id, err)
			continue
		}
		if err := db.Model(&model.Inbound{}).Where("id = ?", inbound.Id).
			Update("settings", string(newSettings)).Error; err != nil {
			return err
		}
		repaired++
	}
	if repaired > 0 {
		log.Printf("Removed duplicate client entries from %d inbound(s)", repaired)
	}
	return nil
}

func isIgnorableDuplicateColumnErr(gdb *gorm.DB, err error, mdl any) bool {
	if err == nil {
		return false
	}
	errMsg := strings.ToLower(err.Error())

	const sqlitePrefix = "duplicate column name:"
	if _, after, ok := strings.Cut(errMsg, sqlitePrefix); ok {
		col := strings.TrimSpace(after)
		col = strings.Trim(col, "`\"[]")
		return col != "" && gdb != nil && gdb.Migrator().HasColumn(mdl, col)
	}
	if strings.Contains(errMsg, "already exists") && strings.Contains(errMsg, "column ") {
		if _, after, ok := strings.Cut(errMsg, "column \""); ok {
			rest := after
			if e := strings.Index(rest, "\""); e > 0 {
				col := rest[:e]
				return col != "" && gdb != nil && gdb.Migrator().HasColumn(mdl, col)
			}
		}
	}
	return false
}

func initUser() error {
	empty, err := isTableEmpty("users")
	if err != nil {
		log.Printf("Error checking if users table is empty: %v", err)
		return err
	}
	if empty {
		hashedPassword, err := crypto.HashPasswordAsBcrypt(defaultPassword)
		if err != nil {
			log.Printf("Error hashing default password: %v", err)
			return err
		}

		user := &model.User{
			Username: defaultUsername,
			Password: hashedPassword,
		}
		return db.Create(user).Error
	}
	return nil
}

func runSeeders(isUsersEmpty bool) error {
	empty, err := isTableEmpty("history_of_seeders")
	if err != nil {
		log.Printf("Error checking if users table is empty: %v", err)
		return err
	}

	if empty && isUsersEmpty {
		seeders := []string{"UserPasswordHash", "ClientsTable", "InboundClientsArrayFix", "InboundClientSubIdFix", "FreedomFinalRulesReverseFix", "FreedomFinalRulesPrivateEgressBlock", "InboundRealityFinalmaskTcpStrip", "WireguardPeersToClients", "MtprotoSecretsToClients"}
		for _, name := range seeders {
			if err := db.Create(&model.HistoryOfSeeders{SeederName: name}).Error; err != nil {
				return err
			}
		}
		return nil
	}

	var seedersHistory []string
	if err := db.Model(&model.HistoryOfSeeders{}).Pluck("seeder_name", &seedersHistory).Error; err != nil {
		log.Printf("Error fetching seeder history: %v", err)
		return err
	}

	if !slices.Contains(seedersHistory, "UserPasswordHash") && !isUsersEmpty {
		var users []model.User
		if err := db.Find(&users).Error; err != nil {
			log.Printf("Error fetching users for password migration: %v", err)
			return err
		}

		for _, user := range users {
			if crypto.IsHashed(user.Password) {
				continue
			}
			hashedPassword, err := crypto.HashPasswordAsBcrypt(user.Password)
			if err != nil {
				log.Printf("Error hashing password for user '%s': %v", user.Username, err)
				return err
			}
			if err := db.Model(&user).Update("password", hashedPassword).Error; err != nil {
				log.Printf("Error updating password for user '%s': %v", user.Username, err)
				return err
			}
		}

		hashSeeder := &model.HistoryOfSeeders{
			SeederName: "UserPasswordHash",
		}
		if err := db.Create(hashSeeder).Error; err != nil {
			return err
		}
	}

	if !slices.Contains(seedersHistory, "ClientsTable") {
		if err := seedClientsFromInboundJSON(); err != nil {
			return err
		}
	}

	if !slices.Contains(seedersHistory, "InboundClientsArrayFix") {
		if err := normalizeInboundClientsArray(); err != nil {
			return err
		}
	}

	if !slices.Contains(seedersHistory, "InboundClientSubIdFix") {
		if err := normalizeInboundClientSubId(); err != nil {
			return err
		}
	}

	if !slices.Contains(seedersHistory, "FreedomFinalRulesReverseFix") {
		if err := normalizeFreedomFinalRules(); err != nil {
			return err
		}
	}

	if !slices.Contains(seedersHistory, "FreedomFinalRulesPrivateEgressBlock") {
		if err := hardenFreedomFinalRules(); err != nil {
			return err
		}
	}

	if !slices.Contains(seedersHistory, "InboundRealityFinalmaskTcpStrip") {
		if err := stripRealityFinalmaskTcp(); err != nil {
			return err
		}
	}

	if err := seedWireguardPeersToClients(); err != nil {
		return err
	}

	// Self-gated on the "MtprotoSecretsToClients" row.
	if err := seedMtprotoSecretsToClients(); err != nil {
		return err
	}

	// Self-gated on the "StripMtprotoInboundSecrets" row. Must run after the
	// seeder above so legacy single-secret inbounds are first converted to a
	// client (which preserves the secret) before the inbound-level copy is
	// dropped from every mtproto inbound.
	if err := stripMtprotoInboundSecrets(); err != nil {
		return err
	}

	// Idempotent, not seeder-gated: restored backups may contain a malformed
	// panel base path.
	return normalizeSettingPaths()
}

func clearLegacyProxySettings() error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("key IN ?", []string{"panelProxy", "tgBotProxy"}).
			Delete(&model.Setting{}).Error; err != nil {
			return err
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "LegacyProxySettingsCleanup"}).Error
	})
}

func normalizeSettingPaths() error {
	pathKeys := []string{"webBasePath"}
	var rows []model.Setting
	if err := db.Where("key IN ?", pathKeys).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		fixed := row.Value
		if !strings.HasPrefix(fixed, "/") {
			fixed = "/" + fixed
		}
		if !strings.HasSuffix(fixed, "/") {
			fixed += "/"
		}
		if fixed == row.Value {
			continue
		}
		if err := db.Model(&model.Setting{}).Where("id = ?", row.Id).
			Update("value", fixed).Error; err != nil {
			return err
		}
	}
	return nil
}

func normalizeInboundClientSubId() error {
	var inbounds []model.Inbound
	if err := db.Find(&inbounds).Error; err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for _, inbound := range inbounds {
			if strings.TrimSpace(inbound.Settings) == "" {
				continue
			}
			var settings map[string]any
			if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
				log.Printf("InboundClientSubIdFix: skip inbound %d (invalid settings json): %v", inbound.Id, err)
				continue
			}
			clients, ok := settings["clients"].([]any)
			if !ok {
				continue
			}
			mutated := false
			for i, raw := range clients {
				obj, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				existing, _ := obj["subId"].(string)
				if strings.TrimSpace(existing) != "" {
					continue
				}
				obj["subId"] = random.NumLower(16)
				clients[i] = obj
				mutated = true
			}
			if !mutated {
				continue
			}
			settings["clients"] = clients
			newSettings, err := json.MarshalIndent(settings, "", "  ")
			if err != nil {
				log.Printf("InboundClientSubIdFix: skip inbound %d (marshal failed): %v", inbound.Id, err)
				continue
			}
			if err := tx.Model(&model.Inbound{}).Where("id = ?", inbound.Id).
				Update("settings", string(newSettings)).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "InboundClientSubIdFix"}).Error
	})
}

func normalizeInboundClientsArray() error {
	var inbounds []model.Inbound
	if err := db.Find(&inbounds).Error; err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for _, inbound := range inbounds {
			if strings.TrimSpace(inbound.Settings) == "" {
				continue
			}
			var settings map[string]any
			if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
				log.Printf("InboundClientsArrayFix: skip inbound %d (invalid settings json): %v", inbound.Id, err)
				continue
			}
			raw, exists := settings["clients"]
			if !exists || raw != nil {
				continue
			}
			settings["clients"] = []any{}
			newSettings, err := json.MarshalIndent(settings, "", "  ")
			if err != nil {
				log.Printf("InboundClientsArrayFix: skip inbound %d (marshal failed): %v", inbound.Id, err)
				continue
			}
			if err := tx.Model(&model.Inbound{}).Where("id = ?", inbound.Id).
				Update("settings", string(newSettings)).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "InboundClientsArrayFix"}).Error
	})
}

func normalizeFreedomFinalRules() error {
	var setting model.Setting
	err := db.Model(model.Setting{}).Where("key = ?", "xrayTemplateConfig").First(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&model.HistoryOfSeeders{SeederName: "FreedomFinalRulesReverseFix"}).Error
	}
	if err != nil {
		return err
	}

	updated, changed, rErr := rewriteFreedomFinalRules(setting.Value)
	if rErr != nil {
		log.Printf("FreedomFinalRulesReverseFix: skip (invalid xrayTemplateConfig json): %v", rErr)
		return db.Create(&model.HistoryOfSeeders{SeederName: "FreedomFinalRulesReverseFix"}).Error
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if changed {
			if err := tx.Model(&model.Setting{}).Where("key = ?", "xrayTemplateConfig").
				Update("value", updated).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "FreedomFinalRulesReverseFix"}).Error
	})
}

func rewriteFreedomFinalRules(raw string) (string, bool, error) {
	if strings.TrimSpace(raw) == "" {
		return raw, false, nil
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return raw, false, err
	}
	outbounds, ok := cfg["outbounds"].([]any)
	if !ok {
		return raw, false, nil
	}
	changed := false
	for _, ob := range outbounds {
		obj, ok := ob.(map[string]any)
		if !ok {
			continue
		}
		if proto, _ := obj["protocol"].(string); proto != "freedom" {
			continue
		}
		settings, ok := obj["settings"].(map[string]any)
		if !ok {
			continue
		}
		if !isLegacyPrivateOnlyFinalRules(settings["finalRules"]) {
			continue
		}
		settings["finalRules"] = []any{map[string]any{"action": "allow"}}
		changed = true
	}
	if !changed {
		return raw, false, nil
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return raw, false, err
	}
	return string(out), true, nil
}

func isLegacyPrivateOnlyFinalRules(v any) bool {
	rules, ok := v.([]any)
	if !ok || len(rules) != 1 {
		return false
	}
	rule, ok := rules[0].(map[string]any)
	if !ok {
		return false
	}
	if action, _ := rule["action"].(string); action != "allow" {
		return false
	}
	ips, ok := rule["ip"].([]any)
	if !ok || len(ips) != 1 {
		return false
	}
	if s, _ := ips[0].(string); s != "geoip:private" {
		return false
	}
	for k := range rule {
		if k != "action" && k != "ip" {
			return false
		}
	}
	return true
}

func hardenFreedomFinalRules() error {
	var setting model.Setting
	err := db.Model(model.Setting{}).Where("key = ?", "xrayTemplateConfig").First(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&model.HistoryOfSeeders{SeederName: "FreedomFinalRulesPrivateEgressBlock"}).Error
	}
	if err != nil {
		return err
	}

	updated, changed, rErr := rewriteFreedomFinalRulesPrivateEgress(setting.Value)
	if rErr != nil {
		log.Printf("FreedomFinalRulesPrivateEgressBlock: skip (invalid xrayTemplateConfig json): %v", rErr)
		return db.Create(&model.HistoryOfSeeders{SeederName: "FreedomFinalRulesPrivateEgressBlock"}).Error
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if changed {
			if err := tx.Model(&model.Setting{}).Where("key = ?", "xrayTemplateConfig").
				Update("value", updated).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "FreedomFinalRulesPrivateEgressBlock"}).Error
	})
}

func rewriteFreedomFinalRulesPrivateEgress(raw string) (string, bool, error) {
	if strings.TrimSpace(raw) == "" {
		return raw, false, nil
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return raw, false, err
	}
	outbounds, ok := cfg["outbounds"].([]any)
	if !ok {
		return raw, false, nil
	}
	changed := false
	for _, ob := range outbounds {
		obj, ok := ob.(map[string]any)
		if !ok {
			continue
		}
		if proto, _ := obj["protocol"].(string); proto != "freedom" {
			continue
		}
		settings, ok := obj["settings"].(map[string]any)
		if !ok {
			continue
		}
		if !isAllowOnlyFinalRules(settings["finalRules"]) && !isLegacyPrivateOnlyFinalRules(settings["finalRules"]) {
			continue
		}
		settings["finalRules"] = []any{
			map[string]any{"action": "block", "ip": []any{"geoip:private"}},
			map[string]any{"action": "allow"},
		}
		changed = true
	}
	if !changed {
		return raw, false, nil
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return raw, false, err
	}
	return string(out), true, nil
}

func stripRealityFinalmaskTcp() error {
	var inbounds []model.Inbound
	if err := db.Find(&inbounds).Error; err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for i := range inbounds {
			updated, changed := stripRealityFinalmaskTcpFromStream(inbounds[i].StreamSettings)
			if !changed {
				continue
			}
			if err := tx.Model(&model.Inbound{}).Where("id = ?", inbounds[i].Id).
				Update("stream_settings", updated).Error; err != nil {
				return err
			}
			log.Printf("InboundRealityFinalmaskTcpStrip: removed finalmask.tcp from REALITY inbound %d (%s)", inbounds[i].Id, inbounds[i].Tag)
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "InboundRealityFinalmaskTcpStrip"}).Error
	})
}

func stripRealityFinalmaskTcpFromStream(raw string) (string, bool) {
	if strings.TrimSpace(raw) == "" {
		return raw, false
	}
	var stream map[string]any
	if err := json.Unmarshal([]byte(raw), &stream); err != nil {
		return raw, false
	}
	if sec, _ := stream["security"].(string); sec != "reality" {
		return raw, false
	}
	finalmask, ok := stream["finalmask"].(map[string]any)
	if !ok {
		return raw, false
	}
	if tcp, _ := finalmask["tcp"].([]any); len(tcp) == 0 {
		return raw, false
	}
	delete(finalmask, "tcp")
	if len(finalmask) == 0 {
		delete(stream, "finalmask")
	}
	out, err := json.Marshal(stream)
	if err != nil {
		return raw, false
	}
	return string(out), true
}

func isAllowOnlyFinalRules(v any) bool {
	rules, ok := v.([]any)
	if !ok || len(rules) != 1 {
		return false
	}
	rule, ok := rules[0].(map[string]any)
	if !ok {
		return false
	}
	if action, _ := rule["action"].(string); action != "allow" {
		return false
	}
	for k := range rule {
		if k != "action" {
			return false
		}
	}
	return true
}

func normalizeClientJSONFields(obj map[string]any) {
	normalizeInt := func(key string) {
		raw, exists := obj[key]
		if !exists {
			return
		}
		s, ok := raw.(string)
		if !ok {
			return
		}
		trimmed := strings.ReplaceAll(strings.TrimSpace(s), " ", "")
		if trimmed == "" {
			delete(obj, key)
			return
		}
		if n, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			obj[key] = n
		} else {
			delete(obj, key)
		}
	}
	for _, k := range []string{"tgId", "limitIp", "totalGB", "expiryTime", "reset", "created_at", "updated_at"} {
		normalizeInt(k)
	}
}

func seedClientsFromInboundJSON() error {
	var inbounds []model.Inbound
	if err := db.Find(&inbounds).Error; err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		byEmail := map[string]*model.ClientRecord{}

		var existing []model.ClientRecord
		if err := tx.Find(&existing).Error; err != nil {
			return err
		}
		for i := range existing {
			byEmail[existing[i].Email] = &existing[i]
		}

		for _, inbound := range inbounds {
			if strings.TrimSpace(inbound.Settings) == "" {
				continue
			}
			var settings map[string]any
			if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
				log.Printf("ClientsTable seed: skip inbound %d (invalid settings json): %v", inbound.Id, err)
				continue
			}
			rawList, ok := settings["clients"].([]any)
			if !ok {
				continue
			}

			for _, raw := range rawList {
				obj, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				normalizeClientJSONFields(obj)
				blob, err := json.Marshal(obj)
				if err != nil {
					continue
				}
				var c model.Client
				if err := json.Unmarshal(blob, &c); err != nil {
					log.Printf("ClientsTable seed: skip client in inbound %d (unmarshal failed): %v; payload=%s",
						inbound.Id, err, string(blob))
					continue
				}
				email := strings.TrimSpace(c.Email)
				if email == "" {
					continue
				}
				incoming := c.ToRecord()

				row, dup := byEmail[email]
				if !dup {
					if err := tx.Create(incoming).Error; err != nil {
						return err
					}
					byEmail[email] = incoming
					row = incoming
				} else {
					conflicts := model.MergeClientRecord(row, incoming)
					for _, x := range conflicts {
						log.Printf("client merge: email=%s conflict on %s old=%v new=%v kept=%v",
							email, x.Field, x.Old, x.New, x.Kept)
					}
					if err := tx.Save(row).Error; err != nil {
						return err
					}
				}

				link := model.ClientInbound{
					ClientId:     row.Id,
					InboundId:    inbound.Id,
					FlowOverride: c.Flow,
				}
				if err := tx.Where("client_id = ? AND inbound_id = ?", row.Id, inbound.Id).
					FirstOrCreate(&link).Error; err != nil {
					return err
				}
			}
		}

		return tx.Create(&model.HistoryOfSeeders{SeederName: "ClientsTable"}).Error
	})
}

func isTableEmpty(tableName string) (bool, error) {
	var count int64
	err := db.Table(tableName).Count(&count).Error
	return count == 0, err
}

func InitDB(dbPath string) error {
	var gormLogger logger.Interface
	if config.IsDebug() {
		gormLogger = logger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags),
			logger.Config{
				SlowThreshold:             time.Second,
				LogLevel:                  logger.Info,
				IgnoreRecordNotFoundError: true,
				Colorful:                  true,
			},
		)
	} else {
		gormLogger = logger.Discard
	}
	c := &gorm.Config{Logger: gormLogger, DisableForeignKeyConstraintWhenMigrating: true}

	dir := path.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	synchronous := sqliteSynchronous()
	journal := sqliteJournalMode()
	dsn := dbPath + "?_journal_mode=" + journal + "&_busy_timeout=10000&_synchronous=" + synchronous + "&_txlock=immediate"
	var err error
	db, err = gorm.Open(sqlite.Open(dsn), c)
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	pragmas := []string{
		"PRAGMA journal_mode=" + journal,
		"PRAGMA busy_timeout=10000",
		"PRAGMA synchronous=" + synchronous,
		fmt.Sprintf("PRAGMA cache_size=-%d", envInt("XUI_DB_CACHE_MB", 32)*1024),
		fmt.Sprintf("PRAGMA mmap_size=%d", int64(envInt("XUI_DB_MMAP_MB", 256))*1024*1024),
		"PRAGMA temp_store=MEMORY",
	}
	for _, pragma := range pragmas {
		if _, err := sqlDB.ExecContext(context.Background(), pragma); err != nil {
			return err
		}
	}

	maxOpen := envInt("XUI_DB_MAX_OPEN_CONNS", 8)
	maxIdle := envInt("XUI_DB_MAX_IDLE_CONNS", 4)
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(30 * time.Minute)

	if err := initModels(); err != nil {
		return err
	}

	isUsersEmpty, err := isTableEmpty("users")
	if err != nil {
		return err
	}

	if err := initUser(); err != nil {
		return err
	}
	if err := runSeeders(isUsersEmpty); err != nil {
		return err
	}
	return MigrateSingleMachine(db)
}

func sqliteJournalMode() string {
	switch strings.ToUpper(strings.TrimSpace(os.Getenv("XUI_DB_JOURNAL_MODE"))) {
	case "DELETE":
		return "DELETE"
	default:
		return "WAL"
	}
}

func sqliteSynchronous() string {
	switch strings.ToUpper(strings.TrimSpace(os.Getenv("XUI_DB_SYNCHRONOUS"))) {
	case "OFF":
		return "OFF"
	case "NORMAL":
		return "NORMAL"
	case "EXTRA":
		return "EXTRA"
	default:
		return "FULL"
	}
}

func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func CloseDB() error {
	if db != nil {
		sqlDB, err := db.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}

func GetDB() *gorm.DB {
	return db
}

func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

func IsSQLiteDB(file io.ReaderAt) (bool, error) {
	signature := []byte("SQLite format 3\x00")
	buf := make([]byte, len(signature))
	_, err := file.ReadAt(buf, 0)
	if err != nil {
		return false, err
	}
	return bytes.Equal(buf, signature), nil
}

func Checkpoint() error {
	return db.Exec("PRAGMA wal_checkpoint(TRUNCATE);").Error
}

func ValidateSQLiteDB(dbPath string) error {
	if _, err := os.Stat(dbPath); err != nil {
		return err
	}
	gdb, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return err
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	var res string
	if err := gdb.Raw("PRAGMA integrity_check;").Scan(&res).Error; err != nil {
		return err
	}
	if res != "ok" {
		return errors.New("sqlite integrity check failed: " + res)
	}
	return nil
}
