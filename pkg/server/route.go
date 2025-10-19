package server

import (
	"github.com/gin-gonic/gin"
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
		articles := apiGroup.Group("/articles")
		{
			articles.GET("", handler.GetArticles)
		}
	}

	return apiGroup
}
