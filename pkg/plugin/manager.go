package plugin

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
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

func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		cfg:    cfg,
		active: make(map[string]bool, len(cfg.Drivers)),
	}
}

// ActivateAll pulls, unpacks, and starts every driver defined in config
func (m *Manager) ActivateAll() error {
	for alias := range m.cfg.Drivers {
		if err := m.ensurePluginRunning(alias); err != nil {
			return fmt.Errorf("failed to start driver %s: %w", alias, err)
		}
	}
	return nil
}

func (m *Manager) ensurePluginRunning(alias string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active[alias] {
		return nil
	}
	drvCfg := m.cfg.Drivers[alias]
	tmpDir := path.Join(m.cfg.CSIEndpointDir, "images", alias)
	socketPath := path.Join(m.cfg.CSIEndpointDir, alias+".sock")

	// pull image to tar file
	tarPath := path.Join(tmpDir, alias+".tar")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return fmt.Errorf("mkdir unpack dir: %w", err)
	}
	image, err := crane.Pull(drvCfg.Image)
	if err != nil {
		return fmt.Errorf("pull image %s: %w", drvCfg.Image, err)
	}
	if err := crane.Save(image, alias, tarPath); err != nil {
		return fmt.Errorf("failed to save image %s to tar: %w", drvCfg.Image, err)
	}

	// extract tar into tmpDir/rootfs
	rootfs := filepath.Join(tmpDir, "rootfs")
	if err := os.MkdirAll(rootfs, 0755); err != nil {
		return fmt.Errorf("mkdir rootfs dir: %w", err)
	}
	if err := unpackTar(tarPath, rootfs); err != nil {
		return fmt.Errorf("untar %s: %w", tarPath, err)
	}

	// locate binary
	binPath := path.Join(rootfs, drvCfg.BinPath)
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		return fmt.Errorf("binary not found at %s", binPath)
	}

	// build args
	args := []string{
		"--nodeid", m.cfg.NodeID,
		"--endpoint", "unix://" + socketPath,
		"--drivername", drvCfg.DriverName,
	}
	args = append(args, drvCfg.StartCommand...)

	cmd := exec.Command(binPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start plugin %s: %w", alias, err)
	}

	// wait for socket
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for socket %s", socketPath)
		}
		time.Sleep(100 * time.Millisecond)
	}

	m.active[alias] = true
	return nil
}

// unpackTar extracts a tar (optionally gzipped) to dest
func unpackTar(tarPath, dest string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()

	var tr *tar.Reader
	if filepath.Ext(tarPath) == ".gz" {
		gr, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gr.Close()
		tr = tar.NewReader(gr)
	} else {
		tr = tar.NewReader(f)
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		target := filepath.Join(dest, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			ns := os.FileMode(hdr.Mode)
			if err := os.MkdirAll(target, ns); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}
