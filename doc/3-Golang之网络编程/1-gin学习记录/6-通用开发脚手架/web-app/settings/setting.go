package settings

import (
	"fmt"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

// 全局变量
var Conf = new(AppConfig)

type AppConfig struct {
	Name         string `mapstructure:"name"`
	Mode         string `mapstructure:"mode"`
	Version      string `mapstructure:"version"`
	Port         int    `mapstructure:"port"`
	*LogConfig   `mapstructure:"log"`
	*MysqlConfig `mapstructure:"mysql"`
	*RedisConfig `mapstructure:"redis"`
}
type LogConfig struct {
	Level      string `mapstructure:"level"`
	FileName   string `mapstructure:"filename"`
	MaxSize    int    `mapstructure:"max_size"`
	MinAge     int    `mapstructure:"min_age"`
	MaxBackups int    `mapstructure:"max_backups"`
}
type MysqlConfig struct {
	Host         string `mapstructure:"host"`
	User         string `mapstructure:"user"`
	Password     string `mapstructure:"password"`
	DbName       string `mapstructure:"db_name"`
	Port         int    `mapstructure:"port"`
	MaxOpenConns int    `mapstructure:"max_open_conns"`
	MaxIdleConns int    `mapstructure:"max_idle_conns"`
}
type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Password string `mapstructure:"password"`
	Port     int    `mapstructure:"port"`
	DB       int    `mapstructure:"db"`
	PoolSize int    `mapstructure:"pool_size"`
}

func Init(fileName string) (err error) {
	// 方式一，相对路径/绝对路径
	// 其中相对路径是指相对于.exe文件
	//viper.SetConfigFile("./config.yaml") // 指定配置文件

	//方式二，指定路径和文件名（不加后缀）
	//config.json/config.yaml等等都可以，
	//只要文件名一致，可以多个相同的文件名，但是不同的后缀，先找到谁就是谁，与后面这个SetConfigType无关
	//viper.SetConfigName("config")
	//viper.AddConfigPath(".") // 相对路径

	// 方式三，命令行参数
	// 使用命令行参数实现方式一
	viper.SetConfigFile(fileName) // 指定配置文件

	// 规定了使用什么格式进行解析
	// 基本搭配远程的配置中心（如etcd），获取字节流之后，使用什么格式进行更新
	// 如果没有搭配远程配置中心，该语句不起作用
	viper.SetConfigType("yaml")

	err = viper.ReadInConfig() // 读取配置信息
	if err != nil {            // 读取配置信息失败
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}

	// 将配置信息反序列化到全局变量中
	if err := viper.Unmarshal(Conf); err != nil {
		fmt.Printf("viper.Unmarshal failed,err:%v\n", err)
	}

	// 监控配置文件变化
	viper.WatchConfig()
	viper.OnConfigChange(func(in fsnotify.Event) {
		fmt.Printf("配置文件发生修改")
		// 更新全局变量中
		if err := viper.Unmarshal(Conf); err != nil {
			fmt.Printf("viper.Unmarshal failed,err:%v\n", err)
		}
	})
	return
}
