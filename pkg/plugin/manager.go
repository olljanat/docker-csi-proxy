// pkg/plugin/manager.go
package plugin

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
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
	tmpDir := filepath.Join("/plugins", alias)
	rootfs := filepath.Join(tmpDir, "rootfs")
	socketPath := filepath.Join("/run", alias+".sock")
	socketPath2 := filepath.Join(tmpDir, "rootfs", socketPath)

	// Download once, only active after that
	if _, err := os.Stat(rootfs); err != nil {
		fmt.Printf("Downloading CSI driver %s\r\n", alias)

		img, err := crane.Pull(drvCfg.Image)
		if err != nil {
			return fmt.Errorf("pull image %s: %w", drvCfg.Image, err)
		}
		tarPath := filepath.Join(tmpDir, alias+".tar")
		if err := os.MkdirAll(tmpDir, 0755); err != nil {
			return fmt.Errorf("mkdir unpack dir: %w", err)
		}
		f, err := os.Create(tarPath)
		if err != nil {
			return fmt.Errorf("could not create tar file %s: %w", tarPath, err)
		}
		defer f.Close()
		if err := crane.Export(img, f); err != nil {
			return fmt.Errorf("failed to export image %s to tar: %w", drvCfg.Image, err)
		}
		if err := os.MkdirAll(rootfs, 0755); err != nil {
			return fmt.Errorf("mkdir rootfs dir: %w", err)
		}
		if err := unpackTar(tarPath, rootfs); err != nil {
			return fmt.Errorf("untar %s: %w", tarPath, err)
		}
	}

	fmt.Printf("Downloading CSI driver %s\r\n", alias)

	// build chrooted command
	// within chroot, binary lives at /<BinPath>
	chrootBin := filepath.Join("/", drvCfg.BinPath)
	args := []string{
		chrootBin,
		"--nodeid", m.cfg.NodeID,
		"--endpoint", "unix://" + socketPath,
		"--drivername", drvCfg.DriverName,
	}
	args = append(args, drvCfg.StartCommand...)

	cmd := exec.Command("chroot", append([]string{rootfs}, args...)...)

	// redirect plugin output to log file
	logDir := filepath.Join(tmpDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("could not create log dir: %w", err)
	}
	logFile := filepath.Join(logDir, alias+".log")
	fLog, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("could not open log file %s: %w", logFile, err)
	}
	// write both stdout and stderr to the same file
	cmd.Stdout = fLog
	cmd.Stderr = fLog
	cmd.SysProcAttr = &syscall.SysProcAttr{}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start chrooted plugin %s: %w", alias, err)
	}

	// wait for socket
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(socketPath2); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for socket %s", socketPath2)
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
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
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
