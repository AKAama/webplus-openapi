package db

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

type Config struct {
	Driver   string `json:"driver,omitempty" yaml:"driver,omitempty"`
	Host     string `json:"host" yaml:"host"`
	Port     int    `json:"port" yaml:"port"`
	Username string `json:"username" yaml:"username"`
	Password string `json:"password" yaml:"password"`
	Database string `json:"database" yaml:"database"`
	Schema   string `json:"schema" yaml:"schema"`
	Debug    bool   `json:"debug" yaml:"debug"`
}

func (t *Config) Validate() []error {
	var errs = make([]error, 0)
	if t.Driver != "" {
		switch strings.ToLower(t.Driver) {
		case "mysql", "postgres", "kingbase":
		default:
			errs = append(errs, errors.Errorf("不支持的数据库驱动: %s", t.Driver))
		}
	}
	if t.Username == "" || t.Password == "" {
		errs = append(errs, errors.Errorf("连接的数据库用户名或密码为空"))
	}
	if t.Database == "" {
		errs = append(errs, errors.Errorf("没有指定需要连接的数据库名称"))
	}
	return errs
}

func NewDefaultDBConfig() *Config {
	return &Config{
		Driver:   "mysql",
		Host:     "127.0.0.1",
		Port:     3306,
		Username: "",
		Password: "",
		Database: "",
		Debug:    false,
	}
}
func (t *Config) DSN() string {
	driver := strings.ToLower(t.Driver)
	switch driver {
	case "postgres", "kingbase":
		return fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=disable TimeZone=Asia/Shanghai search_path=%s", t.Host, t.Username, t.Password, t.Database, t.Port, t.Schema)
	default:
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?%s", t.Username, t.Password, t.Host, t.Port, t.Database, "charset=utf8mb4&parseTime=true&loc=Asia%2fShanghai")
	}
}
