package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/Ferlab-Ste-Justine/systemd-remote/logger"

	yaml "gopkg.in/yaml.v2"
)

type ServerTlsConfig struct {
	CaCert       string `yaml:"ca_cert"`
	ServerCert   string `yaml:"server_cert"`
	ServerKey    string `yaml:"server_key"`
}

type ServerConfig struct{
	Port   int64
	BindIp string           `yaml:"bind_ip"`
	Tls    ServerTlsConfig
}

type Config struct{
	UnitsConfigPath string       `yaml:"units_config_path"`
	LogLevel       string        `yaml:"log_level"`
	Server         ServerConfig
}

func (c *Config) GetLogLevel() int64 {
	logLevel := strings.ToLower(c.LogLevel)
	switch logLevel {
	case "error":
		return logger.ERROR
	case "warning":
		return logger.WARN
	case "debug":
		return logger.DEBUG
	default:
		return logger.INFO
	}
}

func GetConfig(confFilePath string) (Config, error) {
	var c Config

	bs, err := ioutil.ReadFile(confFilePath)
	if err != nil {
		return Config{}, errors.New(fmt.Sprintf("Error reading configuration file: %s", err.Error()))
	}

	err = yaml.Unmarshal(bs, &c)
	if err != nil {
		return Config{}, errors.New(fmt.Sprintf("Error reading configuration file: %s", err.Error()))
	}

	return c, nil
}