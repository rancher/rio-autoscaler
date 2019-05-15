package servicescale

import (
	"context"
	"github.com/sirupsen/logrus"

	"github.com/knative/pkg/logging"
	"github.com/rancher/rio-autoscaler/pkg/logger"
	"github.com/rancher/rio-autoscaler/pkg/metrics"
	"github.com/rancher/rio-autoscaler/types"
	autoscalev1controller "github.com/rancher/rio/pkg/generated/controllers/autoscale.rio.cattle.io/v1"
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

	logrus.Info("Starting ssr controller")
	updator := autoscalev1controller.UpdateServiceScaleRecommendationOnChange(rContext.AutoScale.Autoscale().V1().ServiceScaleRecommendation().Updater(), handler.OnChange)
	rContext.AutoScale.Autoscale().V1().ServiceScaleRecommendation().OnChange(ctx, "ssr-controller", updator)
	rContext.AutoScale.Autoscale().V1().ServiceScaleRecommendation().OnRemove(ctx, "ssr-controller-on-remove", handler.OnRemove)
	return nil
}
