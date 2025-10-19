package cmd

import (
	"context"
	"os"
	"webplus-openapi/pkg/db"
	"webplus-openapi/pkg/nsc"
	"webplus-openapi/pkg/server"
	"webplus-openapi/pkg/signals"
	"webplus-openapi/pkg/store"
	"webplus-openapi/pkg/util"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

func NewServerCommand() *cobra.Command {
	var configFilePath string
	cmd := &cobra.Command{
		Use:   "server",
		Short: "启动api服务",
		Run: func(cmd *cobra.Command, args []string) {
			// 智能设置默认值 ✅
			if configFilePath == "" {
				configFilePath = "./etc/config/config.yaml"
			}

			// 检查文件是否存在 ✅
			_, err := os.Stat(configFilePath)
			os.IsNotExist(err)

			cfg, err := server.TryLoadFromDisk(configFilePath)
			if err != nil {
				zap.S().Errorf("配置文件加载错误:%s", err.Error())
				return
			}
			ctx := signals.SetupSignalHandler()
			_ = startServer(cfg, ctx)
		},
	}
	cmd.PersistentFlags().StringVarP(&configFilePath, "config", "c", "", "配置文件路径")
	return cmd
}

func startServer(cfg *server.Config, ctx context.Context) error {
	zap.S().Infof("***  %s %s ***", util.AppName, util.GetVersion().Version)
	zap.S().Infof("*** 客户ID:%s ***", cfg.ClientName)

	//初始化nats
	if err := nsc.InitNats(cfg.ClientName, cfg.Nats); err != nil {
		zap.S().Fatal(err)
	}

	//初始化mysql
	if err := db.InitDB(cfg.DB); err != nil {
		zap.S().Fatalf("无法连接数据库。%s", err.Error())
	}

	//启动web服务
	webServer := server.NewServer(cfg)

	//初始化业务
	if err := server.Init(cfg); err != nil {
		zap.S().Fatalf("初始化业务失败。%s", err.Error())
	}

	g, c := errgroup.WithContext(ctx)
	g.Go(func() error {
		return webServer.Run()
	})
	//启动nats监听
	g.Go(func() error { return server.GetInstance().Serve(cfg, ctx) })
	g.Go(func() error {
		<-c.Done()
		store.CloseBadgerStore()
		_ = webServer.GracefulShutdown(c)
		return c.Err()
	})
	return g.Wait()

}
