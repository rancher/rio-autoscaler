package service

import (
	"fmt"
	"time"

	"github.com/rancher/rio-autoscaler/pkg/controllers/servicescale"
	"github.com/rancher/rio-autoscaler/types/apis/apps/v1beta1"
	coreclient "github.com/rancher/rio-autoscaler/types/apis/core/v1"
	"github.com/rancher/rio-autoscaler/types/apis/rio-autoscale.cattle.io/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	ssrIndex        = "ssr-index"
	delayAnnotation = "rio-autoscale.cattle.io/delay"
	delay           = 10 * time.Second
)

type serviceHandler struct {
	deployments        v1beta1.DeploymentClient
	endpointsClient    coreclient.EndpointsClient
	endpointsCache     coreclient.EndpointsClientCache
	serviceClient      coreclient.ServiceClient
	serviceClientCache coreclient.ServiceClientCache
	ssrClientCache     v1.ServiceScaleRecommendationClientCache
}

func NewHandler(endpointsClientCache coreclient.EndpointsClientCache,
	serviceClient coreclient.ServiceClient,
	ssrClientCache v1.ServiceScaleRecommendationClientCache) *serviceHandler {
	return &serviceHandler{
		serviceClient:      serviceClient,
		serviceClientCache: serviceClient.Cache(),
		endpointsCache:     endpointsClientCache,
		ssrClientCache:     ssrClientCache,
	}
}

func (s *serviceHandler) AddIndexers(ssrClientCache v1.ServiceScaleRecommendationClientCache) {
	ssrClientCache.Index(ssrIndex, s.indexSSR)
}

func (s *serviceHandler) indexSSR(ssr *v1.ServiceScaleRecommendation) ([]string, error) {
	if ssr.Spec.ServiceNameToRead == "" {
		return nil, nil
	}
	return []string{
		fmt.Sprintf("%s/%s", ssr.Namespace, ssr.Spec.ServiceNameToRead),
	}, nil
}

func (s *serviceHandler) OnSSRChange(ssr *v1.ServiceScaleRecommendation) (runtime.Object, error) {
	if ssr.Status.DesiredScale != nil && *ssr.Status.DesiredScale == 0 {
		s.endpointsClient.Enqueue(ssr.Namespace, ssr.Spec.ServiceNameToRead)
	}
	return ssr, nil
}

func (s *serviceHandler) OnChange(endpoint *corev1.Endpoints) (runtime.Object, error) {
	ssrs, err := s.ssrClientCache.GetIndexed(ssrIndex, fmt.Sprintf("%s/%s", endpoint.Namespace, endpoint.Name))
	if err != nil {
		return nil, err
	}

	for _, ssr := range ssrs {
		if err := s.reconcile(endpoint, ssr); err != nil {
			return endpoint, err
		}
	}

	return endpoint, nil
}

func (s *serviceHandler) updateService(pointToRouter bool, ssr *v1.ServiceScaleRecommendation,
	endpoint *corev1.Endpoints) (*corev1.Service, error) {
	readSvc, err := s.serviceClientCache.Get(ssr.Namespace, ssr.Spec.ServiceNameToRead)
	if err != nil {
		return nil, err
	}

	svc, err := s.serviceClientCache.Get(ssr.Namespace, ssr.Spec.ServiceNameToChange)
	if errors.IsNotFound(err) {
		svc = &corev1.Service{
			TypeMeta: v12.TypeMeta{
				APIVersion: "v1",
				Kind:       "Service",
			},
			ObjectMeta: v12.ObjectMeta{
				Namespace: ssr.Namespace,
				Name:      ssr.Spec.ServiceNameToChange,
			},
			Spec: *readSvc.Spec.DeepCopy(),
		}
		svc.Spec.ClusterIP = ""
		svc, err = s.serviceClient.Create(svc)
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	if pointToRouter && svc.Spec.Type != corev1.ServiceTypeExternalName && ssr.Spec.ZeroScaleService != "" {
		svc = svc.DeepCopy()
		if svc.Annotations == nil {
			svc.Annotations = map[string]string{}
		}
		svc.Annotations[delayAnnotation] = time.Now().Format(time.RFC3339)
		svc.Spec.ExternalName = ssr.Spec.ZeroScaleService
		svc.Spec.Type = corev1.ServiceTypeExternalName
		svc, err = s.serviceClient.Update(svc)
		if err != nil {
			return nil, err
		}
		s.retrigger(endpoint)
	} else if !pointToRouter && svc.Spec.Type != readSvc.Spec.Type {
		svc = svc.DeepCopy()
		delete(svc.Annotations, delayAnnotation)
		svc.Spec.ExternalName = ""
		svc.Spec.Type = readSvc.Spec.Type
		svc, err = s.serviceClient.Update(svc)
		if err != nil {
			return nil, err
		}
	}

	return svc, nil
}

func (s *serviceHandler) retrigger(endpoint *corev1.Endpoints) {
	go func() {
		time.Sleep(delay)
		s.endpointsClient.Enqueue(endpoint.Namespace, endpoint.Name)
	}()
}

func (s *serviceHandler) updateDeploymentScale(pointToRouter bool, ssr *v1.ServiceScaleRecommendation, endpoint *corev1.Endpoints, svc *corev1.Service) error {
	if !pointToRouter {
		return nil
	}

	if ts, ok := svc.Annotations[delayAnnotation]; ok {
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			return err
		}
		if time.Now().Sub(t) >= delay {
			s.retrigger(endpoint)
			return nil
		}
	}

	return servicescale.SetDeploymentScale(s.deployments, ssr, true)
}

func (s *serviceHandler) reconcile(endpoint *corev1.Endpoints, ssr *v1.ServiceScaleRecommendation) error {
	if ssr.Spec.ServiceNameToChange == "" {
		return nil
	}

	pointToRouter := ssr.Status.DesiredScale != nil && *ssr.Status.DesiredScale == 0

	svc, err := s.updateService(pointToRouter, ssr, endpoint)
	if err != nil {
		return err
	}

	return s.updateDeploymentScale(pointToRouter, ssr, endpoint, svc)
}
