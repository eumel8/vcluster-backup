# vcluster backup

A tool to backup periodically your sqlite DB from K3S/vCluster to S3 storage.

## prerequisites

- [vCluster](https://www.vcluster.com/docs/getting-started/deployment) deployed in non-HA with K3S and embedded sqlite DB
- S3 compatible storage, using [Minio with security fixes](https://github.com/eumel8/minio/tree/fix/securitycontext/helm/minio)
- bring the tool into the K3S pod # TODO: use a sidecar container to the vcluster-pod

```bash
tar -cf - vcluster-backup | kubectl -n kunde2 exec --stdin kunde2-vcluster-0 -- sh -c "cat > /tmp/vcluster-backup.tar"
kubectl -n kunde2 exec -it kunde2-vcluster-0 -- sh
cd /tmp
tar xf vcluster-backup.tar
```


## usage

On a single Kubernetes cluster build with vCluster and K3S there is no mechanism included to backup your cluster or your backend database. Of course, there are hints to use RDS or etcd, in our use case we have the embedded sqlite, which is in fact one file what we want to backup securely and periodically. 


```bash
./vcluster-backup -h
Usage of ./vcluster-backup:
  -accessKey string
    	S3 accesskey.
  -backupFile string
    	Sqlite database of K3S instance. (default "/data/server/db/state.db")
  -backupInterval int
    	Interval in minutes for backup. (default 2)
  -bucketName string
    	S3 bucket name. (default "k3s-backup")
  -decrypt
    	Decrypt the file
  -encKey string
    	S3 encryption key.
  -endpoint string
    	S3 endpoint.
  -list
    	List S3 objects
  -region string
    	S3 region. (default "default")
  -secretKey string
    	S3 secretkey.
  -trace
    	Trace S3 API calls

```

start backup:

```bash
./vcluster-backup -accessKey vclusterbackup99 -bucketName vclusterbackup99 -endpoint vcluster-backup.minio.io -secretKey xxxxxx -encKey 12345 -backupInterval 1
# TODO: we need the /data/server/token?
```

list backups:

```bash
./vcluster-backup -accessKey vclusterbackup99 -bucketName vclusterbackup99 -endpoint vcluster-backup.minio.io -secretKey xxxxxx -list
Listing S3 objects in bucket  vclusterbackup99
Object: backup_20240304143145.db.enc
Object: backup_20240304143245.db.enc
Object: backup_20240304143345.db.enc
Object: backup_20240304144757.db.enc
Object: backup_20240304144858.db.enc
Object: backup_20240304150748.db.enc
Object: backup_20240304150848.db.enc
```

restore backup:

```bash
# stop k3s server
rm -rf /data/server/*
mkdir -p /data/server/db
./vcluster-backup -accessKey vclusterbackup99 -bucketName vclusterbackup99 -endpoint vcluster-backup.minio.io -secretKey xxxxxx -backupFile backup_20240304143345.db.enc -encKey 12345 -restore
cp backup_20240304143345.db.enc-restore /data/server/db/state.db
# start k3s server
```

## build

```bash
go mod tidy
CGO_ENABLED=0 go build -o vcluster-backup vcluster-backup.go
```
