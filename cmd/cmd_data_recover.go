package cmd

import (
	"fmt"
	"webplus-openapi/pkg/db"
	"webplus-openapi/pkg/recover"
	"webplus-openapi/pkg/store"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func NewRecoverCommand() *cobra.Command {
	var (
		siteID         string // 站点ID
		columnID       string // 栏目ID
		batchSize      int    // 批次大小
		concurrency    int    // 并发数
		workerPoolSize int    // Worker池大小
	)

	cmd := &cobra.Command{
		Use:   "recover",
		Short: "修复历史数据",
		RunE: func(cmd *cobra.Command, args []string) error {
			// 获取配置文件路径
			configFilePath := cmd.Root().PersistentFlags().Lookup("config").Value.String()

			// 加载配置
			cfg, err := recover.TryLoadFromDisk(configFilePath)
			if err != nil {
				zap.S().Errorf("配置文件加载错误:%s", err.Error())
				return fmt.Errorf("无法加载配置文件: %w", err)
			}

			// 恢复文章数据
			params := recover.Params{
				SiteID:         siteID,
				ColumnID:       columnID,
				BatchSize:      batchSize,
				Concurrency:    concurrency,
				WorkerPoolSize: workerPoolSize,
			}

			return runHistoryDataRecover(cfg, params)
		},
	}

	// 数据恢复相关参数
	cmd.Flags().StringVar(&siteID, "siteId", "", "站点ID (空字符串表示所有站点)")
	cmd.Flags().StringVar(&columnID, "columnId", "", "栏目ID (空字符串表示所有栏目)")
	cmd.Flags().IntVar(&batchSize, "batchSize", 500, "批次大小")
	cmd.Flags().IntVar(&concurrency, "concurrency", 0, "并发数 (0表示使用CPU核心数)")
	cmd.Flags().IntVar(&workerPoolSize, "workerPoolSize", 0, "Worker池大小 (0表示使用并发数的2倍)")

	return cmd
}

func runHistoryDataRecover(cfg *recover.Config, params recover.Params) error {
	// 1. 初始化数据库连接
	if err := db.InitDB(cfg.DB); err != nil {
		zap.S().Errorf("数据库初始化失败. [%s]", err.Error())
		return fmt.Errorf("数据库初始化失败: %w", err)
	}
	zap.S().Info("数据库初始化成功")

	// 2. 获取数据库连接实例
	webplusDB := db.GetDB()
	if webplusDB == nil {
		errMsg := "数据库连接不可用"
		zap.S().Error(errMsg)
		return errors.New(errMsg)
	}

	// 3. 初始化BadgerHold存储
	bs := store.GetBadgerStore()
	badgerStore := store.NewBadgerStoreAdapter(bs)
	zap.S().Info("BadgerHold存储初始化成功")

	// 4. 初始化Manager（单例模式）
	if err := recover.Init(cfg); err != nil {
		zap.S().Errorf("Manager初始化失败: %s", err.Error())
		return fmt.Errorf("Manager初始化失败: %w", err)
	}
	manager := recover.GetInstance() // 获取单例实例

	// 5. 创建恢复服务
	recoverService := recover.NewRecoverService(webplusDB, manager, badgerStore)

	// 6. 输出恢复参数信息
	zap.S().Infof("恢复参数: SiteID=%s, ColumnID=%s, BatchSize=%d, Concurrency=%d, WorkerPoolSize=%d",
		params.SiteID, params.ColumnID, params.BatchSize, params.Concurrency, params.WorkerPoolSize)

	// 7. 执行历史数据恢复
	if err := recoverService.RecoverHistoryData(params); err != nil {
		zap.S().Errorf("历史数据恢复失败: %s", err.Error())
		return fmt.Errorf("历史数据恢复失败: %w", err)
	}

	zap.S().Info("历史数据恢复任务成功完成")
	return nil
}
