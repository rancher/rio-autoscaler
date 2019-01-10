package servicescale

import (
	"context"
	"fmt"
	"sync"

	kpa "github.com/knative/serving/pkg/apis/autoscaling/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/autoscaler"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/autoscaling"
	corev1client "github.com/rancher/rio-autoscaler/types/apis/core/v1"
	"github.com/rancher/rio/types/apis/rio-autoscale.cattle.io/v1"
	riov1 "github.com/rancher/rio/types/apis/rio.cattle.io/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var SyncMap sync.Map

type ssrHandler struct {
	ctx               context.Context
	metrics           autoscaling.KPAMetrics
	pollers           map[string]*poller
	pollerLock        sync.Mutex
	rioServices       riov1.ServiceClient
	rioServicesLister riov1.ServiceClientCache
	services          corev1client.ServiceClientCache
	pods              corev1client.PodClientCache
}

func NewHandler(ctx context.Context, metrics autoscaling.KPAMetrics,
	rioServiceClient riov1.ServiceClient,
	serviceClientCache corev1client.ServiceClientCache,
	podClientCache corev1client.PodClientCache) *ssrHandler {

	return &ssrHandler{
		ctx:               ctx,
		metrics:           metrics,
		pollers:           map[string]*poller{},
		rioServices:       rioServiceClient,
		rioServicesLister: rioServiceClient.Cache(),
		services:          serviceClientCache,
		pods:              podClientCache,
	}
}

func (s *ssrHandler) OnChange(ssr *v1.ServiceScaleRecommendation) (runtime.Object, error) {
	m, err := s.createMetric(ssr)
	if err != nil {
		return ssr, err
	}

	s.monitor(ssr)

	ssr.Status.DesiredScale = bounded(m.DesiredScale, ssr.Spec.MinScale, ssr.Spec.MaxScale)
	return ssr, SetDeploymentScale(s.rioServices, s.rioServicesLister, ssr)
}

func bounded(value, lower, upper int32) *int32 {
	if value < lower {
		return &lower
	}
	if upper > 0 && value > upper {
		return &upper
	}
	return &value
}

func (s *ssrHandler) OnRemove(ssr *v1.ServiceScaleRecommendation) (runtime.Object, error) {
	s.pollerLock.Lock()
	defer s.pollerLock.Unlock()

	key := key(ssr)
	p := s.pollers[key]
	if p != nil {
		p.Stop()
	}

	delete(s.pollers, key)
	return ssr, s.deleteMetric(ssr)
}

func (s *ssrHandler) deleteMetric(ssr *v1.ServiceScaleRecommendation) error {
	key := key(ssr)
	return s.metrics.Delete(s.ctx, key)
}

func (s *ssrHandler) createMetric(ssr *v1.ServiceScaleRecommendation) (*autoscaler.Metric, error) {
	key := key(ssr)
	metric, err := s.metrics.Get(s.ctx, key)
	if err != nil && errors.IsNotFound(err) {
		return s.metrics.Create(s.ctx, toKPA(ssr))
	} else if err != nil {
		return nil, err
	}
	return metric, nil
}

func SetDeploymentScale(rioServiceClient riov1.ServiceClient, rioServiceClientCache riov1.ServiceClientCache, ssr *v1.ServiceScaleRecommendation) error {
	svc, err := rioServiceClientCache.Get(ssr.Namespace, ssr.Name)
	if err != nil {
		return err
	}

	current := svc.Spec.Scale

	if current != int(*ssr.Status.DesiredScale) {
		if current == 1 && *ssr.Status.DesiredScale == 0 {
			synced, ok := SyncMap.Load(key(ssr))
			if ok && synced.(bool) {
				return nil
			}
		}

		svc = svc.DeepCopy()

		svc.Spec.Scale = int(*ssr.Status.DesiredScale)
		if _, err := rioServiceClient.Update(svc); err != nil {
			return err
		}
	}
	SyncMap.Store(key(ssr), true)

	return nil
}

func (s *ssrHandler) monitor(ssr *v1.ServiceScaleRecommendation) {
	s.pollerLock.Lock()
	defer s.pollerLock.Unlock()

	key := key(ssr)
	p, ok := s.pollers[key]
	if ok {
		p.update(ssr)
		return
	}

	p = newPoller(s.ctx, ssr, s.services, s.pods, func(stat autoscaler.Stat) {
		s.metrics.(*autoscaler.MultiScaler).RecordStat(key, stat)
	})

	s.pollers[key] = p
}

func toKPA(ssr *v1.ServiceScaleRecommendation) *kpa.PodAutoscaler {
	return &kpa.PodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PodAutoscaler",
			APIVersion: kpa.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ssr.Namespace,
			Name:      ssr.Name,
		},
		Spec: kpa.PodAutoscalerSpec{
			ContainerConcurrency: v1alpha1.RevisionContainerConcurrencyType(ssr.Spec.Concurrency),
		},
	}
}

func key(ssr *v1.ServiceScaleRecommendation) string {
	return fmt.Sprintf("%s/%s", ssr.Namespace, ssr.Name)
}
