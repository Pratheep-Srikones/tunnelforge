package config

import (
	"os"
)

func EnsureAllAppPaths() error {
	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}
	logPath, err := GetLogPath()
	if err != nil {
		return err
	}
	err = ensurePathExists(configPath)
	if err != nil {
		return err
	}
	err = ensurePathExists(logPath)
	if err != nil {
		return err
	}
	return nil
}

// Get current App data directory
func getAppDataPath() (string, error) {
	appdataPath, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return appdataPath, nil
}

// Get current App config directory
func GetConfigPath() (string, error) {
	appdataPath, err := getAppDataPath()
	if err != nil {
		return "", err
	}
	return appdataPath + "/" + "tunnel_forge", nil
}

// Get current App log directory
func GetLogPath() (string, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return "", err
	}
	return configPath + "/logs", nil
}

func ensurePathExists(path string) error {
	return os.MkdirAll(path, 0755)
}




