package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

// CfgFile - Global config file path variable (exported for external setting)
var CfgFile string

// Init - Initialize config
func Init() {
	// Set default values
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "text")
	viper.SetDefault("mdadm.path", "/sbin/mdadm")
	viper.SetDefault("lvm.path", "/sbin/lvm")
	viper.SetDefault("parted.path", "/sbin/parted")
	// Load from environment variables
	viper.SetEnvPrefix("OPENHR")
	viper.AutomaticEnv()
	// Load config file
	if CfgFile != "" {
		viper.SetConfigFile(CfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err == nil {
			viper.AddConfigPath(home + "/.openhr")
		}
		viper.AddConfigPath("/etc/openhr")
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintf(os.Stderr, "Using config file: %s\n", viper.ConfigFileUsed())
	}
}

// GetString - Get string config
func GetString(key string) string {
	return viper.GetString(key)
}

// GetInt - Get int config
func GetInt(key string) int {
	return viper.GetInt(key)
}

// GetBool - Get bool config
func GetBool(key string) bool {
	return viper.GetBool(key)
}

// SetConfigFile - Set config file path (kept for backward compatibility)
func SetConfigFile(path string) {
	CfgFile = path
}
