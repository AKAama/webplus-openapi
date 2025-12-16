package db

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

type Config struct {
	Driver          string `json:"driver,omitempty" yaml:"driver,omitempty"`
	Host            string `json:"host" yaml:"host"`
	Port            int    `json:"port" yaml:"port"`
	Username        string `json:"username" yaml:"username"`
	Password        string `json:"password" yaml:"password"`
	Database        string `json:"database" yaml:"database"`
	MaxConnections  int    `json:"maxConnections,omitempty" yaml:"maxConnections,omitempty"`
	MaxIdleConns    int    `json:"maxIdleConns,omitempty" yaml:"maxIdleConns,omitempty"`
	MaxOpenConns    int    `json:"maxOpenConns,omitempty" yaml:"maxOpenConns,omitempty"`
	ConnMaxLifetime int    `json:"connMaxLifetime,omitempty" yaml:"connMaxLifetime,omitempty"`
	Debug           bool   `json:"debug" yaml:"debug"`
}

func (t *Config) Validate() []error {
	var errs = make([]error, 0)
	if t.Driver != "" {
		switch strings.ToLower(t.Driver) {
		case "mysql", "postgres":
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
		Driver:          "mysql",
		Host:            "127.0.0.1",
		Port:            3306,
		Username:        "",
		Password:        "",
		Database:        "",
		MaxConnections:  10,
		MaxIdleConns:    5,
		MaxOpenConns:    20,
		ConnMaxLifetime: 3600, // 1小时
	}
}
func (t *Config) DSN() string {
	driver := strings.ToLower(t.Driver)
	switch driver {
	case "postgres", "postgresql":
		// disable ssl, set timezone
		return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable TimeZone=Asia/Shanghai", t.Host, t.Port, t.Username, t.Password, t.Database)
	default: // mysql
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?%s", t.Username, t.Password, t.Host, t.Port, t.Database, "charset=utf8mb4&parseTime=true&loc=Asia%2fShanghai")
	}
}
