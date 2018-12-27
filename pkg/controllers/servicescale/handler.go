package servicescale

import (
	"context"
	"fmt"
	"sync"

	kpa "github.com/knative/serving/pkg/apis/autoscaling/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/autoscaler"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/autoscaling"
	"github.com/rancher/rio-autoscaler/types/apis/apps/v1beta1"
	corev1client "github.com/rancher/rio-autoscaler/types/apis/core/v1"
	"github.com/rancher/rio-autoscaler/types/apis/rio-autoscale.cattle.io/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type ssrHandler struct {
	ctx               context.Context
	metrics           autoscaling.KPAMetrics
	pollers           map[string]*poller
	pollerLock        sync.Mutex
	deploymentsLister v1beta1.DeploymentClientCache
	deployments       v1beta1.DeploymentClient
	services          corev1client.ServiceClientCache
	pods              corev1client.PodClientCache
}

func NewHandler(ctx context.Context, metrics autoscaling.KPAMetrics,
	depClient v1beta1.DeploymentClient,
	serviceClientCache corev1client.ServiceClientCache,
	podClientCache corev1client.PodClientCache) *ssrHandler {

	return &ssrHandler{
		ctx:               ctx,
		metrics:           metrics,
		pollers:           map[string]*poller{},
		deploymentsLister: depClient.Cache(),
		deployments:       depClient,
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
	return ssr, SetDeploymentScale(s.deployments, ssr, false)
}

func bounded(value, lower, upper int32) int32 {
	if value < lower {
		return lower
	}
	if upper > 0 && value > upper {
		return upper
	}
	return value
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

func SetDeploymentScale(deployments v1beta1.DeploymentClient, ssr *v1.ServiceScaleRecommendation, setZero bool) error {
	if ssr.Spec.DeploymentName != "" {
		return nil
	}

	dep, err := deployments.Cache().Get(ssr.Namespace, ssr.Spec.DeploymentName)
	if err != nil {
		return err
	}

	current := int32(0)
	if dep.Spec.Replicas != nil {
		current = *dep.Spec.Replicas
	}

	if current != ssr.Status.DesiredScale {
		// Never scale to zero, the endpoints controller will do that
		if !setZero && current == 1 && ssr.Status.DesiredScale == 0 {
			return nil
		}

		dep = dep.DeepCopy()
		if ssr.Status.DesiredScale == 0 {
			scale := int32(1)
			dep.Spec.Replicas = &scale
		} else {
			dep.Spec.Replicas = &ssr.Status.DesiredScale
		}

		dep.Spec.Replicas = &ssr.Status.DesiredScale
		_, err := deployments.Update(dep)
		return err
	}

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
			Name:      ssr.Namespace,
		},
		Spec: kpa.PodAutoscalerSpec{
			ContainerConcurrency: v1alpha1.RevisionContainerConcurrencyType(ssr.Spec.Concurrency),
		},
	}
}

func key(ssr *v1.ServiceScaleRecommendation) string {
	return fmt.Sprintf("%s/%s", ssr.Namespace, ssr.Name)
}
