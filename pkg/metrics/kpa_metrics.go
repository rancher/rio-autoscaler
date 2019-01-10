package metrics

import (
	"context"
	"time"

	kpa "github.com/knative/serving/pkg/apis/autoscaling/v1alpha1"
	"github.com/knative/serving/pkg/autoscaler"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/autoscaling"
	"github.com/rancher/rio-autoscaler/pkg/logger"
)

var (
	DefaultConfig = autoscaler.Config{
		ContainerConcurrencyTargetDefault:    1,
		ContainerConcurrencyTargetPercentage: 1.0,
		EnableScaleToZero:                    true,
		MaxScaleUpRate:                       10,
		PanicWindow:                          6 * time.Second,
		ScaleToZeroThreshold:                 8 * time.Minute,
		ScaleToZeroGracePeriod:               4 * time.Minute,
		ScaleToZeroIdlePeriod:                4 * time.Minute,
		StableWindow:                         60 * time.Second,
		TickInterval:                         2 * time.Second,
	}
)

func New(ctx context.Context) autoscaling.KPAMetrics {
	dynConfig := autoscaler.NewDynamicConfig(&DefaultConfig, logger.SugaredLogger)
	return autoscaler.NewMultiScaler(dynConfig, ctx.Done(), uniScalerFactory, logger.SugaredLogger)
}

func uniScalerFactory(kpa *kpa.PodAutoscaler, dynamicConfig *autoscaler.DynamicConfig) (autoscaler.UniScaler, error) {
	return autoscaler.New(dynamicConfig, kpa.Spec.ContainerConcurrency, (*nilReporter)(nil)), nil
}

type nilReporter struct {
}

func (n *nilReporter) Report(m autoscaler.Measurement, v float64) error {
	return nil
}
