package csi

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
)

type Client struct {
	controller csi.ControllerClient
	node       csi.NodeClient
}

func NewClient(endpoint string) (*Client, error) {
	conn, err := grpc.NewClient(endpoint, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("failed to dial CSI endpoint %s: %w", endpoint, err)
	}
	return &Client{
		controller: csi.NewControllerClient(conn),
		node:       csi.NewNodeClient(conn),
	}, nil
}

func (c *Client) CreateVolume(ctx context.Context, name string, params map[string]string) error {
	_, err := c.controller.CreateVolume(ctx, &csi.CreateVolumeRequest{
		Name:       name,
		Parameters: params,
		VolumeCapabilities: []*csi.VolumeCapability{{
			AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{FsType: "ext4"}},
		}},
	})
	return err
}

func (c *Client) DeleteVolume(ctx context.Context, id string) error {
	_, err := c.controller.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: id})
	return err
}

func (c *Client) NodeStageVolume(ctx context.Context, volumeID, stagingTarget string, volumeContext, secrets map[string]string) error {
	fmt.Printf("NodeStageVolume, volume ID: %v , volumeContext: %v\r\n", volumeID, volumeContext)

	_, err := c.node.NodeStageVolume(ctx, &csi.NodeStageVolumeRequest{
		VolumeId:          volumeID,
		StagingTargetPath: stagingTarget,
		VolumeCapability: &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{
					FsType: "ext4",
				},
			},
		},
		VolumeContext: volumeContext,
		Secrets:       secrets,
	})
	return err
}

func (c *Client) NodePublishVolume(ctx context.Context, volumeID, parent string, volumeContext map[string]string) error {
	mount := filepath.Join(parent, "mount")
	staging := filepath.Join(parent, "staging")
	_, err := c.node.NodePublishVolume(ctx, &csi.NodePublishVolumeRequest{
		VolumeId:          volumeID,
		TargetPath:        mount,
		StagingTargetPath: staging,
		VolumeCapability: &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{
					FsType: "ext4",
				},
			},
		},
		VolumeContext: volumeContext,
	})
	return err
}

func (c *Client) NodeUnpublishVolume(ctx context.Context, volumeID, target string) error {
	_, err := c.node.NodeUnpublishVolume(ctx, &csi.NodeUnpublishVolumeRequest{
		VolumeId:   volumeID,
		TargetPath: target,
	})
	return err
}
