package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// InitViperConfig initializes Viper configuration, ensuring app paths exist and reading/creating the config file.
func InitViperConfig() error {
	if err := EnsureAllAppPaths(); err != nil {
		return fmt.Errorf("ensure app paths: %w", err)
	}

	viper.SetConfigName("tunnel_forge")
	viper.SetConfigType("yaml")

	configPath, err := GetConfigPath()
	if err != nil {
		return fmt.Errorf("config path: %w", err)
	}
	viper.AddConfigPath(configPath)

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			filePath := configPath + "/tunnel_forge.yaml"
			if err := viper.WriteConfigAs(filePath); err != nil {
				return fmt.Errorf("could not create config file: %w", err)
			}
		} else {
			return fmt.Errorf("reading config file: %w", err)
		}
	}
	return nil
}

// WriteConfig saves the current Viper configuration to disk.
func WriteConfig() error {
	err := viper.WriteConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok || viper.ConfigFileUsed() == "" {
			configPath, pathErr := GetConfigPath()
			if pathErr != nil {
				return fmt.Errorf("config path: %w", pathErr)
			}
			if err := ensurePathExists(configPath); err != nil {
				return fmt.Errorf("ensure path exists: %w", err)
			}
			return viper.WriteConfigAs(configPath + "/tunnel_forge.yaml")
		}
		return fmt.Errorf("writing config file: %w", err)
	}
	return nil
}

// Set sets the configuration value for a key and persists it to disk.
func Set(key string, val any) error {
	viper.Set(key, val)
	return WriteConfig()
}

// GetString returns the string value associated with the key.
func GetString(key string) string {
	return viper.GetString(key)
}

// GetInt returns the integer value associated with the key.
func GetInt(key string) int {
	return viper.GetInt(key)
}

// GetBool returns the boolean value associated with the key.
func GetBool(key string) bool {
	return viper.GetBool(key)
}

// Get returns the value associated with the key as any.
func Get(key string) any {
	return viper.Get(key)
}

// Delete removes a key from the configuration and persists the changes to disk.
func Delete(key string) error {
	all := viper.AllSettings()
	deleteKeyFromMap(all, strings.Split(key, "."))

	viper.Reset()
	viper.SetConfigName("tunnel_forge")
	viper.SetConfigType("yaml")

	if configPath, err := GetConfigPath(); err == nil {
		viper.AddConfigPath(configPath)
	}
	viper.AutomaticEnv()

	if err := viper.MergeConfigMap(all); err != nil {
		return fmt.Errorf("merge config map: %w", err)
	}
	return WriteConfig()
}

func deleteKeyFromMap(m map[string]any, keys []string) {
	if len(keys) == 0 {
		return
	}
	if len(keys) == 1 {
		delete(m, keys[0])
		return
	}
	if sub, ok := m[keys[0]].(map[string]any); ok {
		deleteKeyFromMap(sub, keys[1:])
	}
}
