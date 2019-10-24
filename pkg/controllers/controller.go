package controllers

import (
	"context"
	"sync"

	"github.com/rancher/rio-autoscaler/pkg/controllers/servicescale"
	"github.com/rancher/rio-autoscaler/types"
)

func Register(ctx context.Context, rContext *types.Context, lock *sync.RWMutex, autoscalers map[string]*servicescale.SimpleScale) error {
	return servicescale.Register(ctx, rContext, lock, autoscalers)
}
