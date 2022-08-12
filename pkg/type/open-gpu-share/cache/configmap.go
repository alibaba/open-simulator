package cache

import (
	corelisters "k8s.io/client-go/listers/core/v1"
	clientgocache "k8s.io/client-go/tools/cache"
)

var (
	ConfigMapLister         corelisters.ConfigMapLister
	ConfigMapInformerSynced clientgocache.InformerSynced
)
