package db

import (
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var gormDB *gorm.DB
var databaseOnce sync.Once

func InitDB(cfg *Config) error {
	var err error
	databaseOnce.Do(func() {

		// 使用 pg 驱动
		dialect := postgres.New(postgres.Config{
			DSN:                  cfg.DSN(),
			PreferSimpleProtocol: true,
		})

		gormDB, err = gorm.Open(dialect, &gorm.Config{
			NowFunc: func() time.Time {
				ti, _ := time.LoadLocation("Asia/Shanghai")
				return time.Now().In(ti)
			},
			Logger: logger.Default.LogMode(logger.Silent),
		})

		if err != nil {
			return
		}

		if cfg.Debug {
			gormDB = gormDB.Debug()
		}

		zap.S().Debug("*** 数据库初始化完成（PG/金仓）***")
	})

	return err
}

func GetDB() *gorm.DB {
	return gormDB
}
