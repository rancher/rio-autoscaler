package servicescale

import (
	"context"
	"fmt"
	"sync"
	"time"

	kpa "github.com/knative/serving/pkg/apis/autoscaling/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/autoscaler"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/autoscaling"
	autoscalev1 "github.com/rancher/rio/pkg/apis/autoscale.rio.cattle.io/v1"
	corev1controller "github.com/rancher/rio/pkg/generated/controllers/core/v1"
	riov1controller "github.com/rancher/rio/pkg/generated/controllers/rio.cattle.io/v1"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var SyncMap sync.Map

type SSRHandler struct {
	ctx         context.Context
	metrics     autoscaling.KPAMetrics
	pollers     map[string]*poller
	pollerLock  sync.Mutex
	rioServices riov1controller.ServiceController
	services    corev1controller.ServiceCache
	pods        corev1controller.PodCache
}

func NewHandler(ctx context.Context, metrics autoscaling.KPAMetrics,
	rioServiceClient riov1controller.ServiceController,
	serviceClientCache corev1controller.ServiceCache,
	podClientCache corev1controller.PodCache) *SSRHandler {

	return &SSRHandler{
		ctx:         ctx,
		metrics:     metrics,
		pollers:     map[string]*poller{},
		rioServices: rioServiceClient,
		services:    serviceClientCache,
		pods:        podClientCache,
	}
}

func (s *SSRHandler) OnChange(key string, ssr *autoscalev1.ServiceScaleRecommendation) (*autoscalev1.ServiceScaleRecommendation, error) {
	m, err := s.createMetric(ssr)
	if err != nil {
		return ssr, err
	}

	s.monitor(ssr)
	logrus.Debugf("Desired scale %v calculated for service %s/%s", m.DesiredScale, ssr.Namespace, ssr.Name)

	ssr.Status.DesiredScale = bounded(m.DesiredScale, ssr.Spec.MinScale, ssr.Spec.MaxScale)
	return ssr, SetDeploymentScale(s.rioServices, ssr)
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

func (s *SSRHandler) OnRemove(key string, ssr *autoscalev1.ServiceScaleRecommendation) (*autoscalev1.ServiceScaleRecommendation, error) {
	s.pollerLock.Lock()
	defer s.pollerLock.Unlock()

	p := s.pollers[key]
	if p != nil {
		p.Stop()
	}

	delete(s.pollers, key)
	return ssr, s.deleteMetric(ssr)
}

func (s *SSRHandler) deleteMetric(ssr *autoscalev1.ServiceScaleRecommendation) error {
	key := key(ssr)
	return s.metrics.Delete(s.ctx, key)
}

func (s *SSRHandler) createMetric(ssr *autoscalev1.ServiceScaleRecommendation) (*autoscaler.Metric, error) {
	key := key(ssr)
	metric, err := s.metrics.Get(s.ctx, key)
	if err != nil && errors.IsNotFound(err) {
		logrus.Infof("creating metrics watcher service %s/%s", ssr.Namespace, ssr.Name)
		return s.metrics.Create(s.ctx, toKPA(ssr))
	} else if err != nil {
		return nil, err
	}
	return metric, nil
}

func SetDeploymentScale(rioServices riov1controller.ServiceController, ssr *autoscalev1.ServiceScaleRecommendation) error {
	svc, err := rioServices.Cache().Get(ssr.Namespace, ssr.Name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	// wait for a minute after scale from zero
	if svc.Status.ScaleFromZeroTimestamp != nil && svc.Status.ScaleFromZeroTimestamp.Add(time.Minute).After(time.Now()) {
		logrus.Infof("skipping setting scale because service  %s/%s is scaled from zero within a minute", svc.Namespace, svc.Name)
		return nil
	}

	observedScale := int(*ssr.Status.DesiredScale)
	if svc.Status.ObservedScale != nil && *svc.Status.ObservedScale == observedScale {
		return nil
	}
	logrus.Infof("Setting desired scale %v for %v/%v", *ssr.Status.DesiredScale, svc.Namespace, svc.Name)

	svc.Status.ObservedScale = &observedScale
	if _, err := rioServices.Update(svc); err != nil {
		return err
	}
	return nil
}

func (s *SSRHandler) monitor(ssr *autoscalev1.ServiceScaleRecommendation) {
	s.pollerLock.Lock()
	defer s.pollerLock.Unlock()

	key := key(ssr)
	p, ok := s.pollers[key]
	if ok {
		p.update(ssr)
		return
	}

	p = newPoller(s.ctx, ssr, s.pods, func(stat autoscaler.Stat) {
		s.metrics.(*autoscaler.MultiScaler).RecordStat(key, stat)
	})

	s.pollers[key] = p
}

func toKPA(ssr *autoscalev1.ServiceScaleRecommendation) *kpa.PodAutoscaler {
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

func key(ssr *autoscalev1.ServiceScaleRecommendation) string {
	return fmt.Sprintf("%s/%s", ssr.Namespace, ssr.Name)
}
