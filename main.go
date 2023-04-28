package main

import (
	"os"

	"github.com/Ferlab-Ste-Justine/systemd-remote/config"
	"github.com/Ferlab-Ste-Justine/systemd-remote/logger"
	"github.com/Ferlab-Ste-Justine/systemd-remote/units"
)

func getEnv(key string, fallback string) string {
    if value, ok := os.LookupEnv(key); ok {
        return value
    }
    return fallback
}

func main() {
	log := logger.Logger{LogLevel: logger.ERROR}

	conf, err := config.GetConfig(getEnv("SYSTEMD_REMOTE_CONFIG_FILE", "config.yml"))
	if err != nil {
		log.Errorf(err.Error())
		os.Exit(1)
	}

	log.LogLevel = conf.GetLogLevel()

	manager := units.UnitsManager{FilePath: conf.UnitsConfigPath, Logger: log}
	loadErr := manager.LoadUnitsConf()
	if loadErr != nil {
		log.Errorf(loadErr.Error())
		os.Exit(1)
	}

	//manager.Apply(map[string]string{})
}