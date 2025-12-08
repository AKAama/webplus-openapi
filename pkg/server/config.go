package server

import (
	"os"
	"path/filepath"
	"strings"
	"webplus-openapi/pkg/db"
	"webplus-openapi/pkg/nsc"
	"webplus-openapi/pkg/util"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

type Config struct {
	ClientName     string                `json:"client_name" yaml:"client_name"`
	Port           int                   `json:"port,omitempty" yaml:"port,omitempty"`
	DB             *db.Config            `json:"db,omitempty" yaml:"db,omitempty"`
	Nats           *nsc.NatsConfig       `json:"nats,omitempty" yaml:"nats,omitempty"`
	ResponseFields *ResponseFieldsConfig `json:"response_fields,omitempty" yaml:"response_fields,omitempty"`
}

// ResponseFieldsConfig 响应字段配置
type ResponseFieldsConfig struct {
	// EnabledFields 启用的字段列表，如果为空则返回所有字段
	EnabledFields []string `json:"enabled_fields,omitempty" yaml:"enabled_fields,omitempty"`
}

func (g *Config) Validate() []error {
	var errs = make([]error, 0)
	if err := util.IsValidPort(g.Port); err != nil {
		errs = append(errs, err)
	}
	if es := g.DB.Validate(); len(es) > 0 {
		errs = append(errs, es...)
	}
	return errs
}

func NewDefaultConfig() *Config {
	return &Config{
		Port: 3000,
		DB:   db.NewDefaultDBConfig(),
		Nats: nsc.NewDefaultNatsConfig(),
	}
}
func TryLoadFromDisk(configFilePath string) (*Config, error) {
	_, err := os.Stat(configFilePath)
	if err != nil {
		return nil, err
	}
	dir, file := filepath.Split(configFilePath)
	fileType := filepath.Ext(file)
	viper.AddConfigPath(dir)
	viper.SetConfigName(strings.TrimSuffix(file, fileType))
	viper.SetConfigType(strings.TrimPrefix(fileType, "."))
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	if err := viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			return nil, err
		}
	}
	cfg := NewDefaultConfig()
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
