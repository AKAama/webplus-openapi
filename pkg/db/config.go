package db

import (
	"fmt"

	"github.com/pkg/errors"
)

type Config struct {
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
	Schema          string `json:"schema" yaml:"schema"`
	DBType          string `json:"dbType" yaml:"dbType"`
}

func (t *Config) Validate() []error {
	var errs = make([]error, 0)
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
	return fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%d sslmode=disable TimeZone=Asia/Shanghai search_path=%s",
		t.Host,
		t.Username,
		t.Password,
		t.Database,
		t.Port,
		t.Schema, // 新增模式字段
	)
}
