package database

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/Uber-Eins/phantom/v3/internal/database/model"
	"github.com/Uber-Eins/phantom/v3/internal/xray"
)

// AutoMigrate must create the client traffic hot-path indexes.
func TestAutoMigrateCreatesHotPathIndexes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.ClientRecord{}, &xray.ClientTraffic{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	cases := []struct {
		model any
		index string
	}{
		{&xray.ClientTraffic{}, "idx_client_traffics_inbound"},
		{&xray.ClientTraffic{}, "idx_client_traffics_renew"},
	}
	for _, c := range cases {
		if !db.Migrator().HasIndex(c.model, c.index) {
			t.Errorf("expected index %q to exist after AutoMigrate", c.index)
		}
	}
}
