/*
Copyright © 2021 Alibaba Group Holding Ltd.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	"context"
	time "time"

	storagev1alpha1 "github.com/alibaba/open-local/pkg/apis/storage/v1alpha1"
	versioned "github.com/alibaba/open-local/pkg/generated/clientset/versioned"
	internalinterfaces "github.com/alibaba/open-local/pkg/generated/informers/externalversions/internalinterfaces"
	v1alpha1 "github.com/alibaba/open-local/pkg/generated/listers/storage/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// NodeLocalStorageInitConfigInformer provides access to a shared informer and lister for
// NodeLocalStorageInitConfigs.
type NodeLocalStorageInitConfigInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha1.NodeLocalStorageInitConfigLister
}

type nodeLocalStorageInitConfigInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
}

// NewNodeLocalStorageInitConfigInformer constructs a new informer for NodeLocalStorageInitConfig type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewNodeLocalStorageInitConfigInformer(client versioned.Interface, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredNodeLocalStorageInitConfigInformer(client, resyncPeriod, indexers, nil)
}

// NewFilteredNodeLocalStorageInitConfigInformer constructs a new informer for NodeLocalStorageInitConfig type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredNodeLocalStorageInitConfigInformer(client versioned.Interface, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.CsiV1alpha1().NodeLocalStorageInitConfigs().List(context.TODO(), options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.CsiV1alpha1().NodeLocalStorageInitConfigs().Watch(context.TODO(), options)
			},
		},
		&storagev1alpha1.NodeLocalStorageInitConfig{},
		resyncPeriod,
		indexers,
	)
}

func (f *nodeLocalStorageInitConfigInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredNodeLocalStorageInitConfigInformer(client, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *nodeLocalStorageInitConfigInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&storagev1alpha1.NodeLocalStorageInitConfig{}, f.defaultInformer)
}

func (f *nodeLocalStorageInitConfigInformer) Lister() v1alpha1.NodeLocalStorageInitConfigLister {
	return v1alpha1.NewNodeLocalStorageInitConfigLister(f.Informer().GetIndexer())
}
