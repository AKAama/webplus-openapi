package util

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Response 统一响应结构
type Response struct {
	Code      int         `json:"code"`
	Message   string      `json:"message,omitempty"`
	Data      interface{} `json:"data,omitempty"`
	Timestamp int64       `json:"timestamp"`
}

// Ok 成功响应
func Ok(c *gin.Context, data interface{}) {
	if data == nil {
		data = gin.H{}
	}

	c.JSON(http.StatusOK, Response{
		Code:      http.StatusOK,
		Message:   "success",
		Data:      data,
		Timestamp: time.Now().Unix(),
	})
}

// Err 错误响应
func Err(c *gin.Context, err interface{}) {
	var message string
	var code = http.StatusInternalServerError

	switch v := err.(type) {
	case error:
		message = v.Error()
	case string:
		message = v
	case gin.H:
		if msg, ok := v["error"].(string); ok {
			message = msg
		}
		if c, ok := v["code"].(int); ok {
			code = c
		}
	default:
		message = "Internal server error"
	}

	c.AbortWithStatusJSON(code, Response{
		Code:      code,
		Message:   message,
		Timestamp: time.Now().Unix(),
	})
}
