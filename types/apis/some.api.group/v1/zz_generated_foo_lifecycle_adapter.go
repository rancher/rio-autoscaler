package v1

import (
	"github.com/rancher/norman/lifecycle"
	"k8s.io/apimachinery/pkg/runtime"
)

type FooLifecycle interface {
	Create(obj *Foo) (runtime.Object, error)
	Remove(obj *Foo) (runtime.Object, error)
	Updated(obj *Foo) (runtime.Object, error)
}

type fooLifecycleAdapter struct {
	lifecycle FooLifecycle
}

func (w *fooLifecycleAdapter) Create(obj runtime.Object) (runtime.Object, error) {
	o, err := w.lifecycle.Create(obj.(*Foo))
	if o == nil {
		return nil, err
	}
	return o, err
}

func (w *fooLifecycleAdapter) Finalize(obj runtime.Object) (runtime.Object, error) {
	o, err := w.lifecycle.Remove(obj.(*Foo))
	if o == nil {
		return nil, err
	}
	return o, err
}

func (w *fooLifecycleAdapter) Updated(obj runtime.Object) (runtime.Object, error) {
	o, err := w.lifecycle.Updated(obj.(*Foo))
	if o == nil {
		return nil, err
	}
	return o, err
}

func NewFooLifecycleAdapter(name string, clusterScoped bool, client FooInterface, l FooLifecycle) FooHandlerFunc {
	adapter := &fooLifecycleAdapter{lifecycle: l}
	syncFn := lifecycle.NewObjectLifecycleAdapter(name, clusterScoped, adapter, client.ObjectClient())
	return func(key string, obj *Foo) (runtime.Object, error) {
		newObj, err := syncFn(key, obj)
		if o, ok := newObj.(runtime.Object); ok {
			return o, err
		}
		return nil, err
	}
}
