package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

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

	gin.SetMode(gin.ReleaseMode)
	engine := gin.Default()

	InitRouter(engine, nil)
	server.srv = &http.Server{
		Addr:    fmt.Sprintf(":%d", server.port),
		Handler: engine,
	}

	return server
}
func (srv *Server) Run() error {
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
