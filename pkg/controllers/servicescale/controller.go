package servicescale

import (
	"context"

	"github.com/rancher/rio-autoscaler/pkg/metrics"
	"github.com/rancher/rio-autoscaler/types/apis/apps/v1beta1"
	v12 "github.com/rancher/rio-autoscaler/types/apis/core/v1"
	"github.com/rancher/rio-autoscaler/types/apis/rio-autoscale.cattle.io/v1"
)

func Register(ctx context.Context) error {
	metrics := metrics.New(ctx)
	autoscaleClients := v1.ClientsFrom(ctx)
	corev1Clients := v12.ClientsFrom(ctx)
	appsv1beta1Clients := v1beta1.ClientsFrom(ctx)

	handler := NewHandler(ctx, metrics, appsv1beta1Clients.Deployment,
		corev1Clients.Service.Cache(), corev1Clients.Pod.Cache())

	autoscaleClients.ServiceScaleRecommendation.OnChange(ctx, "ssr-controller", handler.OnChange)
	autoscaleClients.ServiceScaleRecommendation.OnRemove(ctx, "ssr-controller", handler.OnRemove)
	return nil
}
