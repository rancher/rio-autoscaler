package v1

import (
	"context"

	"github.com/rancher/norman/controller"
	"github.com/rancher/norman/objectclient"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

var (
	FooGroupVersionKind = schema.GroupVersionKind{
		Version: Version,
		Group:   GroupName,
		Kind:    "Foo",
	}
	FooResource = metav1.APIResource{
		Name:         "foos",
		SingularName: "foo",
		Namespaced:   true,

		Kind: FooGroupVersionKind.Kind,
	}
)

type FooList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Foo
}

type FooHandlerFunc func(key string, obj *Foo) (runtime.Object, error)

type FooLister interface {
	List(namespace string, selector labels.Selector) (ret []*Foo, err error)
	Get(namespace, name string) (*Foo, error)
}

type FooController interface {
	Generic() controller.GenericController
	Informer() cache.SharedIndexInformer
	Lister() FooLister
	AddHandler(ctx context.Context, name string, handler FooHandlerFunc)
	AddClusterScopedHandler(ctx context.Context, name, clusterName string, handler FooHandlerFunc)
	Enqueue(namespace, name string)
	Sync(ctx context.Context) error
	Start(ctx context.Context, threadiness int) error
}

type FooInterface interface {
	ObjectClient() *objectclient.ObjectClient
	Create(*Foo) (*Foo, error)
	GetNamespaced(namespace, name string, opts metav1.GetOptions) (*Foo, error)
	Get(name string, opts metav1.GetOptions) (*Foo, error)
	Update(*Foo) (*Foo, error)
	Delete(name string, options *metav1.DeleteOptions) error
	DeleteNamespaced(namespace, name string, options *metav1.DeleteOptions) error
	List(opts metav1.ListOptions) (*FooList, error)
	Watch(opts metav1.ListOptions) (watch.Interface, error)
	DeleteCollection(deleteOpts *metav1.DeleteOptions, listOpts metav1.ListOptions) error
	Controller() FooController
	AddHandler(ctx context.Context, name string, sync FooHandlerFunc)
	AddLifecycle(ctx context.Context, name string, lifecycle FooLifecycle)
	AddClusterScopedHandler(ctx context.Context, name, clusterName string, sync FooHandlerFunc)
	AddClusterScopedLifecycle(ctx context.Context, name, clusterName string, lifecycle FooLifecycle)
}

type fooLister struct {
	controller *fooController
}

func (l *fooLister) List(namespace string, selector labels.Selector) (ret []*Foo, err error) {
	err = cache.ListAllByNamespace(l.controller.Informer().GetIndexer(), namespace, selector, func(obj interface{}) {
		ret = append(ret, obj.(*Foo))
	})
	return
}

func (l *fooLister) Get(namespace, name string) (*Foo, error) {
	var key string
	if namespace != "" {
		key = namespace + "/" + name
	} else {
		key = name
	}
	obj, exists, err := l.controller.Informer().GetIndexer().GetByKey(key)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(schema.GroupResource{
			Group:    FooGroupVersionKind.Group,
			Resource: "foo",
		}, key)
	}
	return obj.(*Foo), nil
}

type fooController struct {
	controller.GenericController
}

func (c *fooController) Generic() controller.GenericController {
	return c.GenericController
}

func (c *fooController) Lister() FooLister {
	return &fooLister{
		controller: c,
	}
}

func (c *fooController) AddHandler(ctx context.Context, name string, handler FooHandlerFunc) {
	c.GenericController.AddHandler(ctx, name, func(key string, obj interface{}) (interface{}, error) {
		if obj == nil {
			return handler(key, nil)
		} else if v, ok := obj.(*Foo); ok {
			return handler(key, v)
		} else {
			return nil, nil
		}
	})
}

func (c *fooController) AddClusterScopedHandler(ctx context.Context, name, cluster string, handler FooHandlerFunc) {
	c.GenericController.AddHandler(ctx, name, func(key string, obj interface{}) (interface{}, error) {
		if obj == nil {
			return handler(key, nil)
		} else if v, ok := obj.(*Foo); ok && controller.ObjectInCluster(cluster, obj) {
			return handler(key, v)
		} else {
			return nil, nil
		}
	})
}

type fooFactory struct {
}

func (c fooFactory) Object() runtime.Object {
	return &Foo{}
}

func (c fooFactory) List() runtime.Object {
	return &FooList{}
}

func (s *fooClient) Controller() FooController {
	s.client.Lock()
	defer s.client.Unlock()

	c, ok := s.client.fooControllers[s.ns]
	if ok {
		return c
	}

	genericController := controller.NewGenericController(FooGroupVersionKind.Kind+"Controller",
		s.objectClient)

	c = &fooController{
		GenericController: genericController,
	}

	s.client.fooControllers[s.ns] = c
	s.client.starters = append(s.client.starters, c)

	return c
}

type fooClient struct {
	client       *Client
	ns           string
	objectClient *objectclient.ObjectClient
	controller   FooController
}

func (s *fooClient) ObjectClient() *objectclient.ObjectClient {
	return s.objectClient
}

func (s *fooClient) Create(o *Foo) (*Foo, error) {
	obj, err := s.objectClient.Create(o)
	return obj.(*Foo), err
}

func (s *fooClient) Get(name string, opts metav1.GetOptions) (*Foo, error) {
	obj, err := s.objectClient.Get(name, opts)
	return obj.(*Foo), err
}

func (s *fooClient) GetNamespaced(namespace, name string, opts metav1.GetOptions) (*Foo, error) {
	obj, err := s.objectClient.GetNamespaced(namespace, name, opts)
	return obj.(*Foo), err
}

func (s *fooClient) Update(o *Foo) (*Foo, error) {
	obj, err := s.objectClient.Update(o.Name, o)
	return obj.(*Foo), err
}

func (s *fooClient) Delete(name string, options *metav1.DeleteOptions) error {
	return s.objectClient.Delete(name, options)
}

func (s *fooClient) DeleteNamespaced(namespace, name string, options *metav1.DeleteOptions) error {
	return s.objectClient.DeleteNamespaced(namespace, name, options)
}

func (s *fooClient) List(opts metav1.ListOptions) (*FooList, error) {
	obj, err := s.objectClient.List(opts)
	return obj.(*FooList), err
}

func (s *fooClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return s.objectClient.Watch(opts)
}

// Patch applies the patch and returns the patched deployment.
func (s *fooClient) Patch(o *Foo, data []byte, subresources ...string) (*Foo, error) {
	obj, err := s.objectClient.Patch(o.Name, o, data, subresources...)
	return obj.(*Foo), err
}

func (s *fooClient) DeleteCollection(deleteOpts *metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return s.objectClient.DeleteCollection(deleteOpts, listOpts)
}

func (s *fooClient) AddHandler(ctx context.Context, name string, sync FooHandlerFunc) {
	s.Controller().AddHandler(ctx, name, sync)
}

func (s *fooClient) AddLifecycle(ctx context.Context, name string, lifecycle FooLifecycle) {
	sync := NewFooLifecycleAdapter(name, false, s, lifecycle)
	s.Controller().AddHandler(ctx, name, sync)
}

func (s *fooClient) AddClusterScopedHandler(ctx context.Context, name, clusterName string, sync FooHandlerFunc) {
	s.Controller().AddClusterScopedHandler(ctx, name, clusterName, sync)
}

func (s *fooClient) AddClusterScopedLifecycle(ctx context.Context, name, clusterName string, lifecycle FooLifecycle) {
	sync := NewFooLifecycleAdapter(name+"_"+clusterName, true, s, lifecycle)
	s.Controller().AddClusterScopedHandler(ctx, name, clusterName, sync)
}
