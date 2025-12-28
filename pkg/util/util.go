package util

import (
	"strings"
	"time"
	"unsafe"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/spf13/cast"
)

func IsValidPort[T int | int32 | uint | uint32 | uint64 | int64 | string](port T) error {
	p, err := cast.ToIntE(port)
	if err != nil {
		return errors.Wrap(err, "端口转换错误")
	}

	if p >= 0 && p < 65535 {
		return nil
	}
	return errors.Errorf("%d不是一个合格的[0-65535]端口", p)
}

func StringToBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

func ParseArticleTime(val string) (time.Time, bool) {
	val = strings.TrimSpace(val)
	if val == "" {
		return time.Time{}, false
	}
	formats := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02",
		"2006/01/02 15:04:05",
		"2006/01/02",
		"2006-01-02T15:04:05-07:00",
		"2006-01-02T15:04:05 -07:00",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, val); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// GetParam 从 Query 或 PostForm 获取参数（优先 Query）
func GetParam(c *gin.Context, key string) string {
	if val := c.Query(key); val != "" {
		return val
	}
	return c.PostForm(key)
}
