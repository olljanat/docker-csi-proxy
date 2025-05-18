# About
Prototype of Docker plugin which allow users to use any [Container Storage Interface (CSI)](https://kubernetes.io/blog/2019/01/15/container-storage-interface-ga/) plugin as backend without Docker Swarm.

At least all CSI plugins mentioned in [CSI plugins for Docker Swarm](https://github.com/olljanat/csi-plugins-for-docker-swarm) should works.

TODO:
* Implement `NodeUnstageVolume`
* Save volumes configuration to disk.
* Automatically download CSI plugins.
* Support multiple CSI plugins in parallel.
* Add Windows support
* Support non-containerized workloads (CSI storage for all)

# Usage
## Extract and run SMB CSI
```bash
docker create --name smbplugin registry.k8s.io/sig-storage/smbplugin:v1.18.0
mkdir -p smb
docker export smbplugin | tar -x -C smb
docker rm -vf smbplugin

mkdir /run/csi-proxy
NODE_ID=$(cat /etc/hostname)
./smbplugin -v=5 --drivername=smb \
  --nodeid=${NODE_ID} \
  --enable-get-volume-stats=false \
  --endpoint="unix:///run/csi-proxy/smb.sock"
```

## Run proxy
```bash
go build
export CSI_ENDPOINT=unix:///run/csi-proxy/smb.sock
./docker-csi-proxy
```

## Create and mount volume
```bash
docker volume create \
  --driver csi-proxy \
  --opt source="//10.10.10.100/data" \
  --opt secret-username="smbuser" \
  --opt secret-password="P@ssw0rd!" \
  my-smb-volume
docker run -it --rm -v my-smb-volume:/data bash
```
