package cmd

import (
	"context"
	stderrors "errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"webplus-openapi/pkg/db"
	"webplus-openapi/pkg/sync"
	"webplus-openapi/pkg/util"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var cfg *sync.Config

func NewSyncCommand() *cobra.Command {
	var configFilePath string
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "站点表、栏目表同步",
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd:   true,
			DisableNoDescFlag:   true,
			DisableDescriptions: true,
			HiddenDefaultCmd:    true,
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			var err error
			cfg, err = sync.TryLoadFromDisk(configFilePath)
			if err != nil {
				return errors.Errorf("读取本地配置文件错误:%s", err.Error())
			}
			if errs := cfg.Validate(); len(errs) > 0 {
				return errors.Errorf("本地配置文件验证错误:%s", stderrors.Join(errs...))
			}
			// 初始化源库
			if cfg.SourceDB != nil {
				if err := db.InitSourceDB(cfg.SourceDB); err != nil {
					return errors.Errorf("初始化源库失败:%s", err.Error())
				}
			}
			// 初始化目标库
			if cfg.TargetDB != nil {
				if err := db.InitTargetDB(cfg.TargetDB); err != nil {
					return errors.Errorf("初始化目标库失败:%s", err.Error())
				}
			}

			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			if err := runSyncServer(cfg); err != nil {
				zap.S().Errorf(err.Error())
				return
			}
		},
		Version: util.GetVersion().Version,
	}
	cmd.PersistentFlags().StringVarP(&configFilePath, "config", "c", "./etc/config.yaml", "配置文件路径")
	_ = cmd.MarkPersistentFlagRequired("config")
	return cmd
}

func runSyncServer(cfg *sync.Config) error {
	if cfg == nil {
		return stderrors.New("配置为空")
	}

	// 获取数据库连接
	sourceDB := db.GetSourceDB()
	targetDB := db.GetTargetDB()

	if sourceDB == nil {
		return stderrors.New("源库未初始化")
	}
	if targetDB == nil {
		return stderrors.New("目标库未初始化")
	}

	// 创建同步服务
	columnSyncService := sync.NewColumnSyncServiceWithDB(sourceDB, targetDB)
	siteSyncService := sync.NewSiteSyncServiceWithDB(sourceDB, targetDB)
	publishSiteSyncService := sync.NewPublishSiteSyncServiceWithDB(sourceDB, targetDB)

	// 设置优雅关闭
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// 如果配置了启动时立即执行
	if cfg.Schedule != nil && cfg.Schedule.RunOnStart {
		zap.S().Info("启动时立即执行一次同步...")
		// 同步 T_COLUMN
		if err := columnSyncService.SyncColumns(); err != nil {
			zap.S().Errorf("启动时同步 T_COLUMN 失败: %v", err)
		} else {
			zap.S().Info("启动时同步 T_COLUMN 完成")
		}
		// 同步 T_SITE
		if err := siteSyncService.SyncSites(); err != nil {
			zap.S().Errorf("启动时同步 T_SITE 失败: %v", err)
		} else {
			zap.S().Info("启动时同步 T_SITE 完成")
		}
		// 同步 T_PUBLISHSITE
		if err := publishSiteSyncService.SyncNow(); err != nil {
			zap.S().Errorf("启动时同步 T_PUBLISHSITE 失败: %v", err)
		} else {
			zap.S().Info("启动时同步 T_PUBLISHSITE 完成")
		}
	}

	// 启动定时同步
	if cfg.Schedule != nil && cfg.Schedule.Cron != "" {
		// 如果配置了 cron 表达式，使用 cron 调度
		zap.S().Infof("使用 cron 表达式启动定时同步: %s", cfg.Schedule.Cron)
		// 启动 T_COLUMN 同步
		if err := columnSyncService.StartCronSync(ctx, cfg); err != nil {
			return fmt.Errorf("启动 T_COLUMN cron 定时同步失败: %w", err)
		}
		// 启动 T_SITE 同步
		if err := siteSyncService.StartCronSync(ctx, cfg); err != nil {
			return fmt.Errorf("启动 T_SITE cron 定时同步失败: %w", err)
		}
		// 启动 T_PUBLISHSITE 同步
		if err := publishSiteSyncService.StartCronSync(ctx, cfg); err != nil {
			return fmt.Errorf("启动 T_PUBLISHSITE cron 定时同步失败: %w", err)
		}
	} else {
		// 默认每天12点执行
		zap.S().Info("启动每日定时同步（每天12点执行）")
		// 启动 T_COLUMN 同步
		if err := columnSyncService.StartDailySync(ctx); err != nil {
			return fmt.Errorf("启动 T_COLUMN 每日定时同步失败: %w", err)
		}
		// 启动 T_SITE 同步
		if err := siteSyncService.StartDailySync(ctx); err != nil {
			return fmt.Errorf("启动 T_SITE 每日定时同步失败: %w", err)
		}
		// 启动 T_PUBLISHSITE 同步
		if err := publishSiteSyncService.StartDailySync(ctx); err != nil {
			return fmt.Errorf("启动 T_PUBLISHSITE 每日定时同步失败: %w", err)
		}
	}

	// 等待退出信号
	zap.S().Info("同步服务已启动，等待退出信号...")
	<-sigChan
	zap.S().Info("接收到退出信号，正在关闭同步服务...")
	cancel()

	return nil
}
