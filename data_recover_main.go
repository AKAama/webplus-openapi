package main

import (
	"os"
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
	command := cmd.NewRecoverCommand()
	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}
