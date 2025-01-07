package config

import (
	"my-tshirt-shop/internal/pkg/psql"
	"os"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/joho/godotenv"
)

const (
	envPath = ".env"
)

type App struct {
	DBConfig *psql.Config `yaml:"db"`
}

func NewAppConfig() (*App, error) {
	if err := godotenv.Load(envPath); err != nil {
		return nil, err
	}
	cfgApp := new(App)
	if err := cleanenv.ReadConfig(os.Getenv("CONFIG_PATH"), cfgApp); err != nil {
		return nil, err
	}
	return cfgApp, nil
}
