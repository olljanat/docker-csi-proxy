package config

import (
	"log"
	"os"
)

type Config struct {
	CSIEndpoint string
}

func LoadConfig() *Config {
	endpoint := os.Getenv("CSI_ENDPOINT")
	if endpoint == "" {
		log.Fatal("CSI_ENDPOINT environment variable is required")
	}
	return &Config{
		CSIEndpoint: endpoint,
	}
}
