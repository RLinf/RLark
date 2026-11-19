package db

import (
	"encoding/json"
	"fmt"
	"time"

	"go.yaml.in/yaml/v2"
)

// Config holds PostgreSQL connection configuration.
type Config struct {
	Host     string `json:"host" yaml:"host"`
	Port     int    `json:"port" yaml:"port"`
	Database string `json:"database" yaml:"database"`
	User     string `json:"user" yaml:"user"`
	Password string `json:"password" yaml:"password"`

	MaxOpenConns    int           `json:"maxOpenConns" yaml:"maxOpenConns"`
	MaxIdleConns    int           `json:"maxIdleConns" yaml:"maxIdleConns"`
	ConnMaxLifetime time.Duration `json:"connMaxLifetime" yaml:"connMaxLifetime"`
	ConnMaxIdleTime time.Duration `json:"connMaxIdleTime" yaml:"connMaxIdleTime"`
	// Debug enables query logging when true.
	Debug bool `json:"debug" yaml:"debug"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Host:            "localhost",
		Port:            5432,
		Database:        "rlark",
		User:            "rlark",
		Password:        "rlark",
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 30 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,
	}
}

// DSN returns the PostgreSQL DSN string used by pgdriver.
func (c Config) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		c.User, c.Password, c.Host, c.Port, c.Database,
	)
}

// DSNWithSSL returns the DSN with configurable SSL mode.
func (c Config) DSNWithSSL(sslmode string) string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.Database, sslmode,
	)
}

// UnmarshalConfig unmarshals the config.
func UnmarshalConfig(data []byte, cfg *Config) error {
	if err := yaml.Unmarshal(data, cfg); err == nil {
		return nil
	}
	if err := json.Unmarshal(data, cfg); err == nil {
		return nil
	}
	return fmt.Errorf("failed to unmarshal config as YAML or JSON")
}
