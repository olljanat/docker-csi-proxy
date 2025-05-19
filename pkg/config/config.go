package config

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path"
)

type DriverConfig struct {
	Image        string            `json:"image"`
	DriverName   string            `json:"driverName"`
	Options      map[string]string `json:"options"`
	Secrets      map[string]string `json:"secrets"`
	StartCommand []string          `json:"startCommand"`
	BinPath      string            `json:"binPath"`
}

type Config struct {
	NodeIDEnvVar string                   `json:"nodeIDEnvVar"`
	NodeID       string                   // filled at runtime
	Drivers      map[string]*DriverConfig `json:"drivers"`
}

func LoadConfig() *Config {
	raw, err := ioutil.ReadFile("/etc/docker/csi-proxy.json")
	if err != nil {
		log.Fatalf("could not read config: %v", err)
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		log.Fatalf("invalid config format: %v", err)
	}

	// determine NodeID
	if env := cfg.NodeIDEnvVar; env != "" {
		cfg.NodeID = os.Getenv(env)
	}
	if cfg.NodeID == "" {
		host, _ := os.Hostname()
		cfg.NodeID = host
	}

	return &cfg
}

func (c *Config) SocketFor(alias string) string {
	path := "unix://" + path.Join("/plugins", alias, alias+".sock")
	return path
}
