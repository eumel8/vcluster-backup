# Vcluster backup

A tool to backup periodically your sqlite DB from K3S/vCluster to S3 storage.

## Prerequisites

- [vCluster](https://www.vcluster.com/docs/getting-started/deployment) deployed in non-HA with K3S and embedded sqlite DB
- S3 compatible storage, using [Minio with security fixes](https://github.com/eumel8/minio/tree/fix/securitycontext/helm/minio)

## Usage

On a single Kubernetes cluster build with vCluster and K3S there is no mechanism included to backup your cluster or your backend database. Of course, there are hints to use RDS or etcd, in our use case we have the embedded sqlite, which is in fact one file what we want to backup securely and periodically. 


```bash
$ ./vcluster-backup -h
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
$ ./vcluster-backup -accessKey vclusterbackup99 -bucketName vclusterbackup99 -endpoint minio.example.com -secretKey xxxxxx -encKey 12345 -backupInterval 1
# TODO: we need the /data/server/token?
```

list backups:

```bash
$ ./vcluster-backup -accessKey vclusterbackup99 -bucketName vclusterbackup99 -endpoint minio.example.com -secretKey xxxxxx -list
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
./vcluster-backup -accessKey vclusterbackup99 -bucketName vclusterbackup99 -endpoint minio.example.com -secretKey xxxxxx -backupFile backup_20240304143345.db.enc -encKey 12345 -restore
cp backup_20240304143345.db.enc-restore /data/server/db/state.db
# start k3s server
```


## Integration in vcluster setup

From the idea vcluster-backup is used as sidecar container to the vcluster statefulset. The [origin Helm chart](https://github.com/loft-sh/vcluster/blob/v0.19.3/charts/k3s/templates/syncer.yaml) doesn't support sidecar container. There is a special version with this feature which can be used with this sidecar-values.yaml:

example with env vars and existing secret (prefered method)

<details>
  
```yaml
sidecar:
- env:
    - name: ENDPOINT
      value: minio.example.com
    - name: ACCESS_KEY
      value: vc1
    - name: BUCKET_NAME
      value: vc1
    - name: CLUSTERNAME
      value: vc1
    - name: ENC_KEY
      value: "12345"
    - name: TRACE
      value: "1"
    - name: BACKUP_INTERVAL
      value: "1"
    - name: SECRET_KEY
      valueFrom:
        secretKeyRef:
          name: s3-register
          key: s3secretkey
  image: mtr.devops.telekom.de/caas/vcluster-backup:latest
  imagePullPolicy: Always
  name: backup
  resources:
    limits:
      cpu: "1"
      memory: 512Mi
    requests:
      cpu: 20m
      memory: 64Mi
  securityContext:
    allowPrivilegeEscalation: false
    capabilities:
      drop:
      - all
    readOnlyRootFilesystem: true
    runAsGroup: 1000
    runAsNonRoot: true
    runAsUser: 1000
  volumeMounts:
  - mountPath: /tmp
    name: tmp
  - mountPath: /data
    name: data
```

</details>

example with program flags

<details>

```yaml
sidecar:
  - args:
    - /app/vcluster-backup
    - -endpoint=minio.example.com
    - -accessKey=xxxxx
    - -secretKey=xxxxx
    - -bucketName=vclusterbackups
    - -encKey=12345 
    - -trace=true
    image: mtr.devops.telekom.de/caas/vcluster-backup:latest
    imagePullPolicy: Always
    name: backup
    resources:
      limits:
        cpu: "1"
        memory: 512Mi
      requests:
        cpu: 20m
        memory: 64Mi
    securityContext:
      allowPrivilegeEscalation: false
      capabilities:
        drop:
        - all
      readOnlyRootFilesystem: true
      runAsGroup: 1000
      runAsNonRoot: true
      runAsUser: 1000
    volumeMounts:
    - mountPath: /tmp
      name: tmp
    - mountPath: /data
      name: data
```

</details>

```bash
$ helm -n vc1 upgrade -i vc1 -f sidecar-value.yaml --version v0.19.3 oci://mtr.devops.telekom.de/caas/charts/vcluster
```

## build

```bash
$ go mod tidy
$ CGO_ENABLED=0 go build -o vcluster-backup vcluster-backup.go
```

## Credits

- Frank Kloeker <f.kloeker@telekom.de>

Life is for sharing. If you have an issue with the code or want to improve it, feel free to open an issue or an pull
request.
