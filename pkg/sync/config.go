package sync

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

func TryLoadFromDisk(configFilePath string) (*Config, error) {
	_, err := os.Stat(configFilePath)
	if err != nil {
		return nil, err
	}
	dir, file := filepath.Split(configFilePath)
	fileType := filepath.Ext(file)
	viper.Reset()
	viper.AddConfigPath(dir)
	viper.SetConfigName(strings.TrimSuffix(file, fileType))
	viper.SetConfigType(strings.TrimPrefix(fileType, "."))
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	if err := viper.ReadInConfig(); err != nil {
		if errors.As(err, &viper.ConfigFileNotFoundError{}) {
			return nil, nil
		}
		return nil, err
	}
	cfg := new(Config)
	if err := viper.Unmarshal(cfg, func(dc *mapstructure.DecoderConfig) {
		dc.TagName = strings.TrimPrefix(fileType, ".")
	}); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate 验证配置信息
func (c *Config) Validate() []error {
	var errs = make([]error, 0)
	if c.SourceDB == nil {
		errs = append(errs, errors.New("缺少 db 配置"))
	} else if dbErrs := c.SourceDB.Validate(); len(dbErrs) > 0 {
		errs = append(errs, dbErrs...)
	}
	if c.TargetDB == nil {
		errs = append(errs, errors.New("缺少 dict_db 配置"))
	} else if dictErrs := c.TargetDB.Validate(); len(dictErrs) > 0 {
		errs = append(errs, dictErrs...)
	}

	return errs
}
