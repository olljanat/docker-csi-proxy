package plugin

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/docker/go-plugins-helpers/volume"
	"github.com/olljanat/docker-csi-proxy/pkg/config"
	"github.com/olljanat/docker-csi-proxy/pkg/csi"
)

var (
	baseDir = "/data"
)

type VolumeDriver struct {
	clients map[string]*csi.Client
	cfg     *config.Config
	mgr     *Manager
	volumes map[string]*volumeInfo
	mu      sync.RWMutex
}

type volumeInfo struct {
	Alias   string
	Options map[string]string
	Secrets map[string]string
	Mounted bool
}

// NewVolumeDriver now activates all drivers at startup
func NewVolumeDriver(
	clients map[string]*csi.Client,
	cfg *config.Config,
	mgr *Manager,
) volume.Driver {
	if err := mgr.ActivateAll(); err != nil {
		log.Fatalf("failed to activate CSI plugins: %v", err)
	}
	return &VolumeDriver{
		clients: clients,
		cfg:     cfg,
		mgr:     mgr,
		volumes: make(map[string]*volumeInfo),
	}
}

func (d *VolumeDriver) Create(r *volume.CreateRequest) error {
	alias, ok := r.Options["driver"]
	if !ok {
		return fmt.Errorf("--opt driver=<alias> is required")
	}

	// merge defaults & user opts
	drvCfg := d.cfg.Drivers[alias]
	finalOpts := make(map[string]string)
	for k, v := range drvCfg.Options {
		finalOpts[k] = v
	}
	finalSecrets := make(map[string]string)
	for k, v := range drvCfg.Secrets {
		finalSecrets[k] = v
	}
	for k, v := range r.Options {
		switch k {
		case "driver":
			continue
		default:
			if _, exists := drvCfg.Secrets[k]; exists {
				finalSecrets[k] = v
			} else {
				finalOpts[k] = v
			}
		}
	}

	// point client to correct socket
	// d.clients[alias].SetEndpoint(d.cfg.SocketFor(alias))

	// record and create
	d.mu.Lock()
	d.volumes[r.Name] = &volumeInfo{
		Alias:   alias,
		Options: finalOpts,
		Secrets: finalSecrets,
	}
	d.mu.Unlock()

	parent := filepath.Join("/data", r.Name)
	if _, err := os.Stat(parent); os.IsNotExist(err) {
		if err := os.MkdirAll(parent, os.ModePerm); err != nil {
			log.Fatal(err)
		}
	}
	staging := filepath.Join(baseDir, r.Name, "staging")
	if _, err := os.Stat(staging); os.IsNotExist(err) {
		if err := os.MkdirAll(staging, os.ModePerm); err != nil {
			log.Fatal(err)
		}
	}
	mount := filepath.Join(baseDir, r.Name, "mount")
	if _, err := os.Stat(mount); os.IsNotExist(err) {
		if err := os.MkdirAll(mount, os.ModePerm); err != nil {
			log.Fatal(err)
		}
	}

	return d.clients[alias].CreateVolume(context.Background(), r.Name, finalOpts)
}

func (d *VolumeDriver) Remove(r *volume.RemoveRequest) error {
	v, ok := d.volumes[r.Name]
	if !ok {
		return fmt.Errorf("volume %s doesn not exist", r.Name)
	}
	err := d.clients[v.Alias].DeleteVolume(context.Background(), r.Name)
	if err != nil {
		return err
	}
	d.mu.Lock()
	delete(d.volumes, r.Name)
	d.mu.Unlock()
	return nil
}

func (d *VolumeDriver) Get(r *volume.GetRequest) (*volume.GetResponse, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if _, ok := d.volumes[r.Name]; !ok {
		return nil, fmt.Errorf("volume %s doesn not exist", r.Name)
	}
	return &volume.GetResponse{
		Volume: &volume.Volume{
			Name:       r.Name,
			Mountpoint: "",
		},
	}, nil
}

func (d *VolumeDriver) List() (*volume.ListResponse, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var vols []*volume.Volume
	for name := range d.volumes {
		vols = append(vols, &volume.Volume{Name: name, Mountpoint: ""})
	}
	return &volume.ListResponse{Volumes: vols}, nil
}

func (d *VolumeDriver) Mount(r *volume.MountRequest) (*volume.MountResponse, error) {
	ctx := context.Background()
	d.mu.RLock()
	v := d.volumes[r.Name]
	d.mu.RUnlock()

	if v == nil {
		return nil, fmt.Errorf("volume %s not found", r.Name)
	}
	parent := filepath.Join(baseDir, r.Name)
	mount := filepath.Join(parent, "mount")
	staging := filepath.Join(parent, "staging")

	if !v.Mounted {
		err := d.clients[v.Alias].NodeStageVolume(ctx, r.Name, staging, v.Options, v.Secrets)
		if err != nil {
			return nil, err
		}
		d.mu.Lock()
		v.Mounted = true
		d.mu.Unlock()
	}

	err := d.clients[v.Alias].NodePublishVolume(ctx, r.Name, parent, v.Options)
	if err != nil {
		return nil, err
	}
	return &volume.MountResponse{Mountpoint: mount}, nil
}

func (d *VolumeDriver) Unmount(r *volume.UnmountRequest) error {
	v, ok := d.volumes[r.Name]
	if !ok {
		return fmt.Errorf("volume %s doesn not exist", r.Name)
	}
	mountpoint := filepath.Join(baseDir, r.Name)
	return d.clients[v.Alias].NodeUnpublishVolume(context.Background(), r.Name, mountpoint)
}

func (d *VolumeDriver) Path(r *volume.PathRequest) (*volume.PathResponse, error) {
	path := filepath.Join(baseDir, r.Name)
	return &volume.PathResponse{
		Mountpoint: path,
	}, nil
}

func (d *VolumeDriver) Capabilities() *volume.CapabilitiesResponse {
	return &volume.CapabilitiesResponse{
		Capabilities: volume.Capability{
			Scope: "local",
		},
	}
}
