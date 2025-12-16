package recover

import (
	"os"
	"path/filepath"
	"strings"
	"webplus-openapi/pkg/db"
	"webplus-openapi/pkg/nsc"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

type Config struct {
	SourceDB *db.Config      `json:"source_db,omitempty" yaml:"sourceDB,omitempty"`
	TargetDB *db.Config      `json:"target_db,omitempty" yaml:"targetDB,omitempty"`
	Nats     *nsc.NatsConfig `json:"nats,omitempty" yaml:"nats,omitempty"`
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

func NewDefaultConfig() *Config {
	return &Config{
		Nats:     nsc.NewDefaultNatsConfig(),
		SourceDB: db.NewDefaultDBConfig(),
		TargetDB: db.NewDefaultDBConfig(),
	}
}
