package servicescale

import (
	"context"
	"time"

	"github.com/rancher/norman/pkg/kv"

	"github.com/knative/pkg/logging"
	"github.com/rancher/rio-autoscaler/pkg/logger"
	"github.com/rancher/rio-autoscaler/pkg/metrics"
	v12 "github.com/rancher/rio-autoscaler/types/apis/core/v1"
	"github.com/rancher/rio/types/apis/rio-autoscale.cattle.io/v1"
	riov1 "github.com/rancher/rio/types/apis/rio.cattle.io/v1"
)

func Register(ctx context.Context) error {
	autoscaleClients := v1.ClientsFrom(ctx)
	corev1Clients := v12.ClientsFrom(ctx)
	riov1Clients := riov1.ClientsFrom(ctx)
	metrics := metrics.New(ctx)
	metrics.Watch(func(key string) {
		namespace, name := kv.Split(key, "/")
		SyncMap.Store(key, false)
		autoscaleClients.ServiceScaleRecommendation.Enqueue(namespace, name)
	})

	handler := NewHandler(
		logging.WithLogger(ctx, logger.SugaredLogger),
		metrics,
		riov1Clients.Service,
		corev1Clients.Service.Cache(),
		corev1Clients.Pod.Cache(),
	)

	autoscaleClients.ServiceScaleRecommendation.OnChange(ctx, "ssr-controller", handler.OnChange)
	autoscaleClients.ServiceScaleRecommendation.OnRemove(ctx, "ssr-controller", handler.OnRemove)

	// resetting every 2 minutes
	go func() {
		ticker := time.NewTicker(time.Minute * 5)
		defer ticker.Stop()
		for range ticker.C {
			SyncMap.Range(func(key, value interface{}) bool {
				SyncMap.Store(key, false)
				ns, name := kv.Split(key.(string), "/")
				autoscaleClients.ServiceScaleRecommendation.Enqueue(ns, name)
				return true
			})
		}
	}()

	return nil
}
