package config

import (
	"log"
	"os"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Env          string `yaml:"env" env-required:"true"`
	StoragePath  string `yaml:"storage_path" env-required:"true"`
	AuthPassword string `yaml:"auth_password" env-required:"true"`
	LogfilePath  string `yaml:"logfile_path" evn-required:"true"`
	HTTPServer   `yaml:"http_server"`
}

type HTTPServer struct {
	Address string `yaml:"address" env-default:":8080"`
}

func MustLoad() *Config {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		log.Fatalf("config path is not set, provided path %s:", configPath)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Fatalf("config file does not exist-t: %s", configPath)
	}

	var cfg Config

	if err := cleanenv.ReadConfig(configPath, &cfg); err != nil {
		log.Fatalf("cannot read config: %s", err)
	}

	return &cfg
}
