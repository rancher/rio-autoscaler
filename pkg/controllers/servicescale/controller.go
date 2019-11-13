package servicescale

import (
	"context"
	"sync"

	"github.com/rancher/rio-autoscaler/types"
)

func Register(ctx context.Context, rContext *types.Context, lock *sync.RWMutex, autoscalers map[string]*SimpleScale) error {
	handler := NewHandler(ctx, rContext.Rio.Rio().V1().Service(), rContext.Core.Core().V1().Pod().Cache(), autoscalers, lock)

	rContext.Rio.Rio().V1().Service().OnChange(ctx, "ssr-controller", handler.OnChange)
	return nil
}
