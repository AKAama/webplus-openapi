package db

import (
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var gormSourceDB *gorm.DB
var databaseOnce sync.Once
var gormTargetDB *gorm.DB
var targetOnce sync.Once

// InitDB 初始化源库（支持 mysql/postgres）
func InitDB(cfg *Config) error {
	var err error
	databaseOnce.Do(func() {
		driver := strings.ToLower(cfg.Driver)
		var dial gorm.Dialector
		if driver == "postgres" {
			dial = postgres.Open(cfg.DSN())
		} else {
			dial = mysql.New(mysql.Config{DSN: cfg.DSN()})
		}
		gormSourceDB, err = gorm.Open(dial, &gorm.Config{
			NowFunc: func() time.Time {
				ti, _ := time.LoadLocation("Asia/Shanghai")
				return time.Now().In(ti)
			},
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if cfg.Debug {
			gormSourceDB = gormSourceDB.Debug()
		}
		if err != nil {
			return
		}
		zap.S().Debug("*** 数据库初始化完成 ***")
	})
	return err
}
func GetSourceDB() *gorm.DB {
	return gormSourceDB
}

// InitTargetDB 初始化目标库（强制使用 MySQL，用于 targetDB）
func InitTargetDB(cfg *Config) error {
	var err error
	targetOnce.Do(func() {
		if cfg != nil && cfg.Driver != "" && strings.ToLower(cfg.Driver) != "mysql" {
			zap.S().Warnf("targetDB 强制使用 MySQL，忽略 driver=%s", cfg.Driver)
		}
		dial := mysql.New(mysql.Config{DSN: cfg.DSN()})
		gormTargetDB, err = gorm.Open(dial, &gorm.Config{
			NowFunc: func() time.Time {
				ti, _ := time.LoadLocation("Asia/Shanghai")
				return time.Now().In(ti)
			},
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if cfg != nil && cfg.Debug {
			gormTargetDB = gormTargetDB.Debug()
		}
		if err != nil {
			return
		}
		zap.S().Debug("*** targetDB 数据库初始化完成 (MySQL) ***")
	})
	return err
}

func GetTargetDB() *gorm.DB {
	return gormTargetDB
}
