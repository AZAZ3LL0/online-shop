package psql

import (
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

type Config struct {
	Host     string `yaml:"host"`
	Port     string `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
	SSLMode  string `yaml:"sslmode"`
}

func NewDB(cfg *Config) (*sqlx.DB, error) {
	db, err := sqlx.Open("pgx", fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=%s password=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.DBName, cfg.SSLMode, cfg.Password))
	if err != nil {
		return nil, err
	}
	err = db.Ping()
	if err != nil {
		return nil, err
	}
	return db, nil
}
