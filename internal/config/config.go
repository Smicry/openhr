package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

var cfgFile string

// CfgFile 全局配置文件路径变量
var CfgFile string

// Init 初始化配置
func Init() {
	// 设置默认值
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "text")
	viper.SetDefault("mdadm.path", "/sbin/mdadm")
	viper.SetDefault("lvm.path", "/sbin/lvm")
	viper.SetDefault("parted.path", "/sbin/parted")

	// 从环境变量加载
	viper.SetEnvPrefix("OPENHR")
	viper.AutomaticEnv()

	// 加载配置文件
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
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
		fmt.Fprintf(os.Stderr, "使用配置文件: %s\n", viper.ConfigFileUsed())
	}
}

// GetString 获取字符串配置
func GetString(key string) string {
	return viper.GetString(key)
}

// GetInt 获取整数配置
func GetInt(key string) int {
	return viper.GetInt(key)
}

// GetBool 获取布尔配置
func GetBool(key string) bool {
	return viper.GetBool(key)
}

// SetConfigFile 设置配置文件路径
func SetConfigFile(path string) {
	cfgFile = path
	CfgFile = path
}
