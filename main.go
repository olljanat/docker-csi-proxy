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
	csiClient, err := csi.NewClient(cfg.CSIEndpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create CSI client: %v\n", err)
		os.Exit(1)
	}

	driver := plugin.NewVolumeDriver(csiClient, cfg)
	h := volume.NewHandler(driver)
	fmt.Println("CSI proxy starting")
	if err := h.ServeUnix("csi-proxy", 0); err != nil {
		fmt.Fprintf(os.Stderr, "failed to serve plugin: %v\n", err)
		os.Exit(1)
	}
}
