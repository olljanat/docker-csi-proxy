// pkg/plugin/manager.go
package plugin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/olljanat/docker-csi-proxy/pkg/config"
	"github.com/opencontainers/runtime-spec/specs-go"
)

type Manager struct {
	cfg    *config.Config
	client *containerd.Client
	active map[string]containerd.Container
	mu     sync.Mutex
}

func NewManager(cfg *config.Config) (*Manager, error) {
	cli, err := containerd.New("/run/containerd/containerd.sock",
		containerd.WithDefaultNamespace("csi-proxy"),
	)
	if err != nil {
		return nil, err
	}
	return &Manager{
		cfg:    cfg,
		client: cli,
		active: make(map[string]containerd.Container),
	}, nil
}

// ActivateAll pulls and starts every driver defined in config
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
	if _, ok := m.active[alias]; ok {
		return nil
	}

	drv := m.cfg.Drivers[alias]
	ctx := namespaces.WithNamespace(context.Background(), "csi-proxy")

	// 1) Pull and unpack image
	image, err := m.client.Pull(ctx, drv.Image, containerd.WithPullUnpack)
	if err != nil {
		return fmt.Errorf("pull %s: %w", drv.Image, err)
	}

	// 2) Create containerd container
	// socketHost := filepath.Join(m.cfg.CSIEndpointDir, alias+".sock")
	socketHost := filepath.Join("/run", alias+".sock")
	containerName := "csi-plugin-" + alias
	ctr, err := m.client.NewContainer(
		ctx,
		containerName,
		containerd.WithNewSnapshot(containerName+"-snap", image),
		containerd.WithNewSpec(
			oci.WithImageConfig(image),
			oci.WithHostNamespace(specs.NetworkNamespace),

			oci.WithProcessArgs(
				drv.BinPath,
				"--nodeid", m.cfg.NodeID,
				"--endpoint", "unix://"+socketHost,
				"--drivername", drv.DriverName,
			),
		),
	)
	/*
		oci.WithMounts([]oci.Mount{{
			Type:        "bind",
			Source:      m.cfg.CSIEndpointDir,
			Destination: m.cfg.CSIEndpointDir,
			Options:     []string{"rbind", "rw"},
		}}),
	*/
	if err != nil {
		return fmt.Errorf("container create %s: %w", alias, err)
	}

	// 3) Start the container as a task with logging
	tmpDir := filepath.Join("/plugins", alias)
	task, err := ctr.NewTask(ctx, cio.LogFile(filepath.Join(tmpDir, alias+".log")))
	if err != nil {
		return fmt.Errorf("task create %s: %w", alias, err)
	}
	if err := task.Start(ctx); err != nil {
		return fmt.Errorf("task start %s: %w", alias, err)
	}

	// 4) Wait for socket
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(socketHost); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for %s", socketHost)
		}
		time.Sleep(100 * time.Millisecond)
	}

	m.active[alias] = ctr
	return nil
}
