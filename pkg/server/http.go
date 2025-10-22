package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"webplus-openapi/pkg/store"

	"github.com/gin-gonic/gin"
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

	gin.SetMode(gin.DebugMode)
	engine := gin.Default()

	// 创建handler实例
	handler := &Handler{
		cfg:            *cfg,
		articleManager: store.GetBadgerStore(),
	}

	zap.S().Info("开始注册路由...")
	InitRouter(engine, handler)
	zap.S().Info("路由注册完成")

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
