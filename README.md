# Code sever operator
![Publish Docker images](https://github.com/TommyLike/code-server-operator/workflows/Publish%20Docker%20images/badge.svg?branch=stable)

This project used to launch multiple code server instances in k8s cluster.


# Features
1. Release compute resource if the code server keeps inactive for some time.
2. Release volume resource if the code server has not been used for a period of long time.
3. Git clone code automatically before running code sever.

The sample yaml can be found in `example` folder:
```$xslt
apiVersion: cs.tommylike.com/v1alpha1
kind: CodeServer
metadata:
  name: codeserver-tommy
spec:
  url: codeservertommy
  image: "codercom/code-server:v2"
  volumeSize: "200m"
  storageClassName: "local-nfs"
  inactiveAfterSeconds: 600
  recycleAfterSeconds: 1200
  resources:
    requests:
      cpu: "2"
      memory: "2048m"
  serverCipher: "1234"
  initPlugins:
    git:
      - --repourl
      - https://github.com/TommyLike/tommylike.me.git

```

# Develop
We use **kind** to boot up the kubernetes cluster, please use the script file to prepare cluster.
```$xslt
./local_development/local_up.sh
```
export `KUBECONFIG`:
```$xslt
export KUBECONFIG="$(kind get kubeconfig-path development)"
```
generate latest CRD yaml file:
```$xslt
make manifests
```
apply CRD into cluster:
```$xslt
make install
```
then test locally:
```$xslt
make run
```

