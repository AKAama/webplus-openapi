package db

import (
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var gormDB *gorm.DB
var databaseOnce sync.Once

func InitDB(cfg *Config) error {
	var err error
	databaseOnce.Do(func() {
		gormDB, err = gorm.Open(mysql.New(mysql.Config{
			DSN: cfg.DSN(),
		}), &gorm.Config{
			NowFunc: func() time.Time {
				ti, _ := time.LoadLocation("Asia/Shanghai")
				return time.Now().In(ti)
			},
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if cfg.Debug {
			gormDB = gormDB.Debug()
		}
		if err != nil {
			return
		}
		zap.S().Debug("*** 数据库初始化完成 ***")
	})
	return err
}
func GetDB() *gorm.DB {
	return gormDB
}
