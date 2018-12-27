package server

import (
	"context"

	"github.com/rancher/norman"
	"github.com/rancher/norman/types"
	"github.com/rancher/rio-autoscaler/pkg/controllers/servicescale"
	appsclient "github.com/rancher/rio-autoscaler/types/apis/apps/v1beta1"
	coreclient "github.com/rancher/rio-autoscaler/types/apis/core/v1"
	autoscaleclient "github.com/rancher/rio-autoscaler/types/apis/rio-autoscale.cattle.io/v1"
)

func Config() *norman.Config {
	return &norman.Config{
		Name: "rio-autoscaler",
		Schemas: []*types.Schemas{
			autoscaleclient.Schemas,
		},

		CRDs: map[*types.APIVersion][]string{
			&autoscaleclient.APIVersion: {
				autoscaleclient.ServiceScaleRecommendationGroupVersionKind.Kind,
			},
		},

		Clients: []norman.ClientFactory{
			autoscaleclient.Factory,
			coreclient.Factory,
			appsclient.Factory,
		},

		MasterControllers: []norman.ControllerRegister{
			func(ctx context.Context) error {
				return servicescale.Register(ctx)
			},
		},
	}
}
