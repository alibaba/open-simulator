# User's manual

- [User's Manual](# User's Manual)
  - [Cluster Simulation](# Cluster Simulation)
    - [Build Virtual Cluster](# Build Virtual Cluster)
    - [Copy Existing Cluster](#Copy Existing Cluster)
  - [Simulate Deployment Application](# Simulate Deployment Application)
    - [General Application](#General Application)
    - [Chart application](#chart application)

## Cluster simulation

The command line description of > simon is described in [link](. /commandline/simon.md)

### Building a virtual cluster

Edit [example/simon-config.yaml](... /... /example/simon-config.yaml) file to set up the custom cluster.

```yaml
apiVersion: simon/v1alpha1
kind: Config
metadata:
  name: simon-config
spec:
  cluster:
    customConfig: example/cluster/demo_1
The ```

The .spec.cluster.customConfig field contains a folder path containing the files necessary to build the virtual cluster:

- Cluster node information. The node yaml file is stored in the example/cluster/demo_1/nodes folder
- Cluster initial container resources
  - Static Pods (e.g. kube-scheduler, kube-apiserver, etc.). Pod yaml files are stored in the manifests folder of example/cluster/demo_1
  - General resources. yaml files are stored in the example/cluster/demo_1 folder

Execute the command to see the simulated virtual cluster.

```bash
bin/simon apply -i -f example/simon-config.yaml
```

### Replicate an existing cluster

Edit [example/simon-config.yaml](... /... /example/simon-config.yaml) file and set the kubeconfig file path.

```yaml
apiVersion: simon/v1alpha1
kind: Config
metadata:
  name: simon-config
spec:
  cluster:
    kubeConfig: /root/.kube/config
```

The .spec.cluster.kubeConfig field is filled with the absolute path to the kubeconfig file of the real k8s cluster.

Execute the command to see the replicated virtual cluster.

```bash
bin/simon apply -i -f example/simon-config.yaml
```

## Simulate deploying the application

### Common application

Edit [example/simon-config.yaml](... /... /example/simon-config.yaml) file.

```yaml
apiVersion: simon/v1alpha1
kind: Config
metadata:
  name: simon-config
spec:
  cluster:
    customConfig: example/cluster/demo_1
  appList:
    - name: simple
      path: example/application/simple
    - name: complicated
      path: example/application/complicate
  newNode: example/newnode/demo_1
```

Prepare the application yaml files to be deployed (in this case the files are stored in the example/application/simple and example/application/complicate directories) and populate them as an array in the .spec.cluster.appList field.

Also, in case the resources of the virtual cluster do not meet the deployment conditions of the application, you need to prepare a node to be added in the .spec.cluster.newNode field.

Execute the command to see the application deployed in the virtual cluster.

```bash
bin/simon apply -i -f example/simon-config.yaml
```

### Chart application

Edit [example/simon-config.yaml](... /... /example/simon-config.yaml) file.

```yaml
apiVersion: simon/v1alpha1
kind: Config
metadata:
  name: simon-config
spec:
  cluster:
    customConfig: example/cluster/demo_1
  appList:
    - name: yoda
      path: example/application/charts/yoda
      chart: true
  newNode: example/newnode/demo_1
```

Prepare the Chart file for the application to be deployed (in this case, the file is saved in the example/application/charts/yoda directory) and fill it into the .spec.cluster.appList field as an array, and note that the chart field is set to true (the default is false).

Execute the command and you will see the Chart application deployed in the virtual cluster.

```bash
bin/simon apply -i -f example/simon-config.yaml
```
