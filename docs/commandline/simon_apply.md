## simon apply

Make a reasonable cluster capacity planning based on application resource requirements

```
simon apply [flags]
```

### Options

```
      --default-scheduler-config string   path to JSON or YAML file containing scheduler configuration.
  -h, --help                              help for apply
  -i, --interactive                       interactive mode
  -f, --simon-config string               path to the cluster kube-config file used to connect cluster, one of both kube-config and cluster-config must exist.
      --use-greed                         use greedy algorithm when queue pods (default true)
```

### Options inherited from parent commands

```
      --log-flush-frequency duration   Maximum number of seconds between log flushes (default 5s)
```

### SEE ALSO

* [simon](simon.md)	 - Simon is a simulator, which will simulate a cluster and simulate workload scheduling.

