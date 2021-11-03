# 模拟 Open-Local 存储调度

当用户创建一个包含 Open-Local PVC 的 Statefulset ，Open-Simulator 会生成对应的 Pod。此时会去掉 Pod 的 Volume 相关信息，并将存储信息写在 pod 的 annotation 中。如果一个 Pod 中有多个 PVC，则自动求和。

举例：

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: nginx-sts
spec:
  replicas: 2
  volumeClaimTemplates:
  - metadata:
      name: html-0
    spec:
      accessModes:
        - ReadWriteOnce
      storageClassName: open-local-lvm
      resources:
        requests:
          storage: 10Gi
  - metadata:
      name: html-1
    spec:
      accessModes:
        - ReadWriteOnce
      storageClassName: open-local-lvm
      resources:
        requests:
          storage: 20Gi
  template:
    spec:
      containers:
      - name: nginx
        image: nginx
        volumeMounts:
        - mountPath: "/data-0"
          name: html-0
        - mountPath: "/data-1"
          name: html-1
```

会根据上面的信息生成 Pod。

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: statefulset-nginx-sts-0
spec:
  containers:
  - name: nginx
    image: nginx
```