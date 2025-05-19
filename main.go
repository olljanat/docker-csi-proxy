package main

import (
	"fmt"
	"os"

	"github.com/docker/go-plugins-helpers/volume"
	"github.com/olljanat/docker-csi-proxy/pkg/config"
	"github.com/olljanat/docker-csi-proxy/pkg/csi"
	"github.com/olljanat/docker-csi-proxy/pkg/plugin"
)

func main() {
	cfg := config.LoadConfig()

	mgr, err := plugin.NewManager(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot start plugin manager: %v\n", err)
		os.Exit(1)
	}

	clients := make(map[string]*csi.Client)
	for alias := range cfg.Drivers {
		endpoint := cfg.SocketFor(alias)
		cli, err := csi.NewClient(endpoint)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to connect to %s: %v\n", alias, err)
			os.Exit(1)
		}
		clients[alias] = cli
	}

	driver := plugin.NewVolumeDriver(clients, cfg, mgr)
	h := volume.NewHandler(driver)
	fmt.Println("CSI proxy starting with drivers:")
	for alias := range clients {
		fmt.Println(" -", alias)
	}
	if err := h.ServeUnix("csi-proxy", 0); err != nil {
		fmt.Fprintf(os.Stderr, "serve plugin: %v\n", err)
		os.Exit(1)
	}
}
