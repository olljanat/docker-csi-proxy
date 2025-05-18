package plugin

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/docker/go-plugins-helpers/volume"
	"github.com/olljanat/docker-csi-proxy/pkg/config"
	"github.com/olljanat/docker-csi-proxy/pkg/csi"
)

var (
	baseDir = "/data"
)

type VolumeDriver struct {
	client  *csi.Client
	cfg     *config.Config
	volumes map[string]*volumeInfo
	mu      sync.RWMutex
}

type volumeInfo struct {
	Name    string
	Options map[string]string
	Secrets map[string]string
	Mounted bool
}

func NewVolumeDriver(c *csi.Client, cfg *config.Config) volume.Driver {
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		if err := os.Mkdir(baseDir, os.ModePerm); err != nil {
			log.Fatal(err)
		}
	}
	return &VolumeDriver{
		client:  c,
		cfg:     cfg,
		volumes: make(map[string]*volumeInfo),
	}
}

func (d *VolumeDriver) Create(r *volume.CreateRequest) error {
	opts, secrets := parseOptions(r.Options)
	volume := &volumeInfo{
		Name:    r.Name,
		Options: opts,
		Secrets: secrets,
		Mounted: false,
	}
	d.mu.Lock()
	d.volumes[r.Name] = volume
	d.mu.Unlock()

	parent := filepath.Join(baseDir, r.Name)
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

	return d.client.CreateVolume(context.Background(), r.Name, opts)
}

func (d *VolumeDriver) Remove(r *volume.RemoveRequest) error {
	d.mu.Lock()
	delete(d.volumes, r.Name)
	d.mu.Unlock()
	return d.client.DeleteVolume(context.Background(), r.Name)
}

func (d *VolumeDriver) Get(r *volume.GetRequest) (*volume.GetResponse, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if _, ok := d.volumes[r.Name]; !ok {
		return nil, fmt.Errorf("volume %s doesn not exist", r.Name)
	}
	return &volume.GetResponse{Volume: &volume.Volume{Name: r.Name, Mountpoint: ""}}, nil
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
		err := d.client.NodeStageVolume(ctx, r.Name, staging, v.Options, v.Secrets)
		if err != nil {
			return nil, err
		}
		d.mu.Lock()
		v.Mounted = true
		d.mu.Unlock()
	}

	err := d.client.NodePublishVolume(ctx, r.Name, parent, v.Options)
	if err != nil {
		return nil, err
	}
	return &volume.MountResponse{Mountpoint: mount}, nil
}

func (d *VolumeDriver) Unmount(r *volume.UnmountRequest) error {
	mountpoint := filepath.Join(baseDir, r.Name)
	return d.client.NodeUnpublishVolume(context.Background(), r.Name, mountpoint)
}

func (d *VolumeDriver) Path(r *volume.PathRequest) (*volume.PathResponse, error) {
	path := fmt.Sprintf("/var/lib/docker-volumes/%s", r.Name)
	return &volume.PathResponse{Mountpoint: path}, nil
}

func (d *VolumeDriver) Capabilities() *volume.CapabilitiesResponse {
	return &volume.CapabilitiesResponse{Capabilities: volume.Capability{Scope: "local"}}
}

func parseOptions(opts map[string]string) (map[string]string, map[string]string) {
	volCtx := make(map[string]string)
	secrets := make(map[string]string)
	for k, v := range opts {
		if strings.HasPrefix(k, "secret-") {
			secret := strings.TrimPrefix(k, "secret-")
			secrets[secret] = v
		} else {
			volCtx[k] = v
		}
	}
	return volCtx, secrets
}
