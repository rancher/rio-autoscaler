package server

import (
	"context"

	"github.com/rancher/norman"
	"github.com/rancher/norman/types"
	"github.com/rancher/rio-autoscaler/pkg/controllers/gateway"
	"github.com/rancher/rio-autoscaler/pkg/controllers/servicescale"
	autoscalerTypes "github.com/rancher/rio-autoscaler/types"
	coreclient "github.com/rancher/rio-autoscaler/types/apis/core/v1"
	"github.com/rancher/rio/types/apis/networking.istio.io/v1alpha3"
	autoscalev1 "github.com/rancher/rio/types/apis/rio-autoscale.cattle.io/v1"
	autoscaleSchema "github.com/rancher/rio/types/apis/rio-autoscale.cattle.io/v1/schema"
	riov1 "github.com/rancher/rio/types/apis/rio.cattle.io/v1"
)

func Config(name string) *norman.Config {
	return &norman.Config{
		Name: name,
		Schemas: []*types.Schemas{
			autoscaleSchema.Schemas,
		},

		CRDs: map[*types.APIVersion][]string{},

		Clients: []norman.ClientFactory{
			autoscalev1.Factory,
			coreclient.Factory,
			riov1.Factory,
			v1alpha3.Factory,
		},

		MasterControllers: []norman.ControllerRegister{
			func(ctx context.Context) error {
				return servicescale.Register(ctx)
			},
			autoscalerTypes.Register(gateway.Register),
		},

		GlobalSetup: autoscalerTypes.BuildContext,
	}
}
