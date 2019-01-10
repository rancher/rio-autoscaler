package types

import (
	"context"

	corev1 "github.com/rancher/rio-autoscaler/types/apis/core/v1"
	"github.com/rancher/rio/types/apis/networking.istio.io/v1alpha3"
	autoscalev1 "github.com/rancher/rio/types/apis/rio-autoscale.cattle.io/v1"
	riov1 "github.com/rancher/rio/types/apis/rio.cattle.io/v1"
)

type contextKey struct{}

type Context struct {
	Core       *corev1.Clients
	Autoscaler *autoscalev1.Clients
	Rio        *riov1.Clients
	Networking *v1alpha3.Clients
}

func Store(ctx context.Context, c *Context) context.Context {
	return context.WithValue(ctx, contextKey{}, c)
}

func From(ctx context.Context) *Context {
	return ctx.Value(contextKey{}).(*Context)
}

func NewContext(ctx context.Context) *Context {
	return &Context{
		Autoscaler: autoscalev1.ClientsFrom(ctx),
		Core:       corev1.ClientsFrom(ctx),
		Rio:        riov1.ClientsFrom(ctx),
		Networking: v1alpha3.ClientsFrom(ctx),
	}
}

func BuildContext(ctx context.Context) (context.Context, error) {
	return Store(ctx, NewContext(ctx)), nil
}

func Register(f func(context.Context, *Context) error) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		return f(ctx, From(ctx))
	}
}
