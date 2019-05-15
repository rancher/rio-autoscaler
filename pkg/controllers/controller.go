package controllers

import (
	"context"
	"github.com/rancher/rio-autoscaler/pkg/controllers/gateway"
	"github.com/rancher/rio-autoscaler/pkg/controllers/servicescale"
	"github.com/rancher/rio-autoscaler/types"
)

func Register(ctx context.Context, rContext *types.Context) error {
	if err := gateway.Register(ctx, rContext); err != nil {
		return err
	}
	return servicescale.Register(ctx, rContext)
}
