package main

import (
	"os"
	"os/signal"
	"syscall"
	"webplus-openapi/cmd"
	"webplus-openapi/pkg/util"

	"go.uber.org/zap"
)

func main() {
	logger := util.InitZapLog()
	zap.ReplaceGlobals(logger)
	defer func(logger *zap.Logger) {
		_ = logger.Sync()
	}(logger)

	// 设置优雅关闭
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		zap.S().Info("接收到关闭信号，正在优雅关闭...")
		os.Exit(0)
	}()

	command := cmd.NewRecoverCommand()
	if err := command.Execute(); err != nil {
		zap.S().Errorf("命令执行失败: %v", err)
		os.Exit(1)
	}

	// 正常退出时也要关闭 Badger
	zap.S().Info("程序正常结束")
}
