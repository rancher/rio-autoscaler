package servicescale

import (
	"context"
	"github.com/rancher/wrangler/pkg/generic"

	"github.com/knative/pkg/logging"
	"github.com/rancher/rio-autoscaler/pkg/logger"
	"github.com/rancher/rio-autoscaler/pkg/metrics"
	"github.com/rancher/rio-autoscaler/types"
	"github.com/rancher/wrangler/pkg/kv"
)

func Register(ctx context.Context, rContext *types.Context) error {
	metrics := metrics.New(ctx)
	metrics.Watch(func(key string) {
		namespace, name := kv.Split(key, "/")
		rContext.AutoScale.Autoscale().V1().ServiceScaleRecommendation().Enqueue(namespace, name)
	})

	handler := NewHandler(
		logging.WithLogger(ctx, logger.SugaredLogger),
		metrics,
		rContext.Rio.Rio().V1().Service(),
		rContext.Core.Core().V1().Service().Cache(),
		rContext.Core.Core().V1().Pod().Cache(),
	)

	rContext.AutoScale.Autoscale().V1().ServiceScaleRecommendation().
		AddGenericHandler(ctx, "ssr-controller", generic.UpdateOnChange(rContext.AutoScale.Autoscale().V1().ServiceScaleRecommendation().Updater(), handler.OnChange))
	rContext.AutoScale.Autoscale().V1().ServiceScaleRecommendation().OnRemove(ctx, "ssr-controller-on-remove", handler.OnRemove)
	return nil
}
