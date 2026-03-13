package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Vault    VaultConfig    `yaml:"vault"`
	Risk     RiskConfig     `yaml:"risk"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type VaultConfig struct {
	Path string `yaml:"path"`
}

type RiskConfig struct {
	MaxRiskPerTrade float64 `yaml:"max_risk_per_trade"`
	MaxDailyLoss    float64 `yaml:"max_daily_loss"`
	MaxPositions    int     `yaml:"max_positions"`
	MaxLeverage     float64 `yaml:"max_leverage"`
	DefaultMode     string  `yaml:"default_mode"`
}

func defaultConfig() *Config {
	return &Config{
		Server:   ServerConfig{Host: "127.0.0.1", Port: 9090},
		Database: DatabaseConfig{Path: "data/clawtrade.db"},
		Vault:    VaultConfig{Path: "data/vault.enc"},
		Risk: RiskConfig{
			MaxRiskPerTrade: 0.02, MaxDailyLoss: 0.05,
			MaxPositions: 5, MaxLeverage: 10, DefaultMode: "paper",
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := defaultConfig()
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
