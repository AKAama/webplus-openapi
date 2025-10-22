package server

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// APIHandler 定义API处理器接口
type APIHandler interface {
	GetArticles(c *gin.Context)
}

// InitRouter 初始化路由配置
func InitRouter(engine *gin.Engine, handler APIHandler) *gin.RouterGroup {
	// API路由组
	apiGroup := engine.Group("/api/v1")
	if handler != nil {
		articles := apiGroup.Group("/webplus")
		{
			articles.GET("/getArticles", handler.GetArticles)
			zap.S().Info("路由注册成功: GET /api/v1/webplus/getArticles")
		}
	} else {
		zap.S().Warn("Handler为nil，路由未注册")
	}

	return apiGroup
}
