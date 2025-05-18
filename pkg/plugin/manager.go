// pkg/plugin/manager.go
package plugin

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/olljanat/docker-csi-proxy/pkg/config"
)

type Manager struct {
	cfg    *config.Config
	active map[string]bool
	mu     sync.Mutex
}

func newManager(cfg *config.Config) *Manager {
	return &Manager{
		cfg:    cfg,
		active: map[string]bool{},
	}
}

// ensurePluginRunning will, on first use of a driver alias:
// 1) crane.Pull the image into /run/csi-proxy/<alias>
// 2) exec the plugin binary from that rootfs, listening on alias.sock
func (m *Manager) ensurePluginRunning(alias string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active[alias] {
		return nil
	}

	drvCfg, ok := m.cfg.Drivers[alias]
	if !ok {
		return fmt.Errorf("unknown CSI driver alias %q", alias)
	}

	tmpDir := path.Join(m.cfg.CSIEndpointDir, "images", alias)
	socketPath := path.Join(m.cfg.CSIEndpointDir, alias+".sock")

	// 1) Pull & unpack OCI image
	//    crane.Pull will fetch all layers and extract their filesystem contents into tmpDir.
	if err := os.RemoveAll(tmpDir); err != nil {
		return fmt.Errorf("cleanup old image dir: %w", err)
	}
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return fmt.Errorf("mkdir for image unpack: %w", err)
	}
	// You can pass crane.WithAuthFromKeychain(...) here if you need private registries.
	if err := crane.Pull(drvCfg.Image, tmpDir); err != nil {
		return fmt.Errorf("failed to pull+extract image %s: %w", drvCfg.Image, err)
	}

	// 2) Find the plugin binary
	//    assume it's named exactly as drvCfg.DriverName + "plugin" under tmpDir/
	binPath := path.Join(tmpDir, drvCfg.DriverName+"plugin")
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		return fmt.Errorf("plugin binary not found at %s", binPath)
	}

	// 3) Launch it
	cmdArgs := []string{
		"--nodeid", m.cfg.NodeID,
		"--endpoint", "unix://" + socketPath,
		"--drivername", drvCfg.DriverName,
	}
	// append any extra flags from config:
	cmdArgs = append(cmdArgs, drvCfg.StartCommand...)
	cmd := exec.Command(binPath, cmdArgs...)
	// detach, so the proxy isn't blocked if the CSI process logs to stdout/stderr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start CSI plugin process: %w", err)
	}

	// give the plugin a moment to create its socket
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for plugin socket at %s", socketPath)
		}
		time.Sleep(100 * time.Millisecond)
	}

	m.active[alias] = true
	return nil
}
