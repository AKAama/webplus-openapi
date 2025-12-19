package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"webplus-openapi/pkg/db"

	_ "webplus-openapi/docs"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/zap"
)

type Server struct {
	srv  *http.Server
	port int
}

func NewServer(cfg *Config) *Server {
	server := &Server{
		port: cfg.Port,
	}

	// 根据环境变量设置Gin模式，默认为Release模式
	ginMode := os.Getenv("GIN_MODE")
	if ginMode == "" {
		ginMode = gin.ReleaseMode
	}
	gin.SetMode(ginMode)
	engine := gin.Default()

	// 创建handler实例（使用 db_storage 中的 MySQL 存储）
	handler := &Handler{
		cfg: *cfg,
		db:  db.GetTargetDB(),
	}

	zap.S().Info("开始注册路由...")
	InitRouter(engine, handler)
	zap.S().Info("路由注册完成")

	engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	server.srv = &http.Server{
		Addr:    fmt.Sprintf(":%d", server.port),
		Handler: engine,
	}

	return server
}
func (srv *Server) Run() error {
	zap.S().Infof("HTTP服务器启动在端口 %d", srv.port)
	err := srv.srv.ListenAndServe()
	if err != nil {
		if errors.Is(err, http.ErrServerClosed) {
			zap.S().Debugf("http server[:%d] 已经关闭...", srv.port)
			return nil
		}
		return err
	}
	return nil
}

func (srv *Server) GracefulShutdown(ctx context.Context) error {
	c, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := srv.srv.Shutdown(c); err != nil {
		zap.S().Errorf("http server 关闭错误:%s", err.Error())
		return err
	}
	return nil
}
