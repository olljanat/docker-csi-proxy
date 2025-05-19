# About
Prototype of Docker plugin which allow users to use any [Container Storage Interface (CSI)](https://kubernetes.io/blog/2019/01/15/container-storage-interface-ga/) plugin as backend without Docker Swarm.

At least all CSI plugins mentioned in [CSI plugins for Docker Swarm](https://github.com/olljanat/csi-plugins-for-docker-swarm) should works.

TODO:
* Implement `NodeUnstageVolume`
* Umount /proc from CSI driver chroots
* Mount /sys and /dev to CSI chroots
* Save volumes configuration to disk.
* Automatically download CSI plugins.
* Support multiple CSI plugins in parallel.
* Add Windows support
* Support non-containerized workloads (CSI storage for all)
* Propagate /data to CSI plugins
* Cleanup in shutdown

# Usage

## Create and mount volume
```bash
docker volume create \
  --driver csi-proxy \
  --opt driver=smb \
  my-smb-volume
docker run -it --rm -v my-smb-volume:/data bash
```

# Troubleshooting
```bash
tail -f /plugins/smb/logs/smb.log
sudo chroot /plugins/smb/rootfs /bin/sh
```

sudo ./nerdctl --namespace=csi-proxy stop $(sudo ./nerdctl --namespace=csi-proxy ps -a -q)
sudo ./nerdctl --namespace=csi-proxy rm $(sudo ./nerdctl --namespace=csi-proxy ps -a -q)
INFO[0000] unable to retrieve networking information for that container  container=csi-plugin-smb error="cannot determine networking options from nil spec.Annotations"
WARN[0000] failed to remove hosts file for container "csi-plugin-smb"  error="hosts-store error\nnot found\nstat /var/lib/nerdctl/1935db59/etchosts/csi-proxy/csi-plugin-smb: no such file or directory"
csi-plugin-s
INFO[0000] unable to retrieve networking information for that container  container=csi-plugin-nfs error="cannot determine networking options from nil spec.Annotations"
WARN[0000] failed to remove hosts file for container "csi-plugin-nfs"  error="hosts-store error\nnot found\nstat /var/lib/nerdctl/1935db59/etchosts/csi-proxy/csi-plugin-nfs: no such file or directory"
csi-plugin-n

olli@ubuntu:/tmp$ docker run -it --rm -v my-smb-volume:/data bash
docker: Error response from daemon: error while mounting volume '': VolumeDriver.Mount: rpc error: code = Internal desc = volume(my-smb-volume) mount "//192.168.8.50/temp2" on "/data/my-smb-volume/staging" failed with mount failed: exit status 2
Mounting command: mount
Mounting arguments: -t cifs -o <masked> //192.168.8.50/temp2 /data/my-smb-volume/staging
Output: Unable to apply new capability set.

