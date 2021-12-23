package settings

import (
	"fmt"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

func Init() (err error) {
	viper.SetConfigFile("config.yaml") // 指定配置文件
	viper.AddConfigPath(".")           // 指定查找配置文件的路径
	err = viper.ReadInConfig()         // 读取配置信息
	if err != nil {                    // 读取配置信息失败
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}
	// 监控配置文件变化
	viper.WatchConfig()
	viper.OnConfigChange(func(in fsnotify.Event) {
		fmt.Printf("配置文件发生修改")
	})
	return
}
