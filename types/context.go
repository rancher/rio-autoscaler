package types

import (
	"context"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	autoscale "github.com/rancher/rio/pkg/generated/controllers/autoscale.rio.cattle.io"
	"github.com/rancher/rio/pkg/generated/controllers/core"
	networking "github.com/rancher/rio/pkg/generated/controllers/networking.istio.io"
	rio "github.com/rancher/rio/pkg/generated/controllers/rio.cattle.io"
	"github.com/rancher/wrangler/pkg/apply"
	"github.com/rancher/wrangler/pkg/start"
)

type contextKey struct{}

type Context struct {
	Namespace string

	Core       *core.Factory
	AutoScale  *autoscale.Factory
	Rio        *rio.Factory
	K8s        kubernetes.Interface
	Networking *networking.Factory

	Apply apply.Apply
}

func Store(ctx context.Context, c *Context) context.Context {
	return context.WithValue(ctx, contextKey{}, c)
}

func From(ctx context.Context) *Context {
	return ctx.Value(contextKey{}).(*Context)
}

func NewContext(namespace string, config *rest.Config) *Context {
	context := &Context{
		Namespace:  namespace,
		AutoScale:  autoscale.NewFactoryFromConfigOrDie(config),
		Core:       core.NewFactoryFromConfigOrDie(config),
		Rio:        rio.NewFactoryFromConfigOrDie(config),
		K8s:        kubernetes.NewForConfigOrDie(config),
		Networking: networking.NewFactoryFromConfigOrDie(config),
	}

	context.Apply = apply.New(context.K8s.Discovery(), apply.NewClientFactory(config))
	return context
}

func (c *Context) Start(ctx context.Context) error {
	return start.All(ctx, 5,
		c.AutoScale,
		c.Core,
		c.Networking,
		c.Rio,
	)
}

func BuildContext(ctx context.Context, namespace string, config *rest.Config) (context.Context, *Context) {
	c := NewContext(namespace, config)
	return context.WithValue(ctx, contextKey{}, c), c
}

func Register(f func(context.Context, *Context) error) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		return f(ctx, From(ctx))
	}
}
