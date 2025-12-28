package server

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// APIHandler 定义API处理器接口
type APIHandler interface {
	GetArticles(c *gin.Context)
	GetColumns(c *gin.Context)
}

// InitRouter 初始化路由配置
func InitRouter(engine *gin.Engine, handler APIHandler) *gin.RouterGroup {
	// API路由组
	apiGroup := engine.Group("/api/v1")
	if handler != nil {
		webplus := apiGroup.Group("/webplus")
		{
			// getArticles 支持 GET 和 POST
			webplus.GET("/getArticles", handler.GetArticles)
			webplus.POST("/getArticles", handler.GetArticles)
			zap.S().Info("路由注册成功: GET/POST /api/v1/webplus/getArticles")

			// getColumns 支持 GET 和 POST
			webplus.GET("/getColumns", handler.GetColumns)
			webplus.POST("/getColumns", handler.GetColumns)
			zap.S().Info("路由注册成功: GET/POST /api/v1/webplus/getColumns")
		}
	} else {
		zap.S().Warn("Handler为nil，路由未注册")
	}

	return apiGroup
}
