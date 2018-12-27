package servicescale

import (
	"context"
	"sync"
	"time"

	"github.com/knative/serving/pkg/autoscaler"
	"github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/rancher/rancher/pkg/ticker"
	"github.com/rancher/rio-autoscaler/pkg/metrics"
	corev1client "github.com/rancher/rio-autoscaler/types/apis/core/v1"
	"github.com/rancher/rio-autoscaler/types/apis/rio-autoscale.cattle.io/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type recorder func(stat autoscaler.Stat)

type poller struct {
	ssrLock sync.Mutex

	ctx      context.Context
	cancel   func()
	ssr      *v1.ServiceScaleRecommendation
	recorder recorder
	services corev1client.ServiceClientCache
	pods     corev1client.PodClientCache

	promAPI promv1.API
	promURL string
}

func newPoller(ctx context.Context, ssr *v1.ServiceScaleRecommendation,
	services corev1client.ServiceClientCache, pods corev1client.PodClientCache, recorder recorder) *poller {
	p := &poller{
		ssr:      ssr,
		recorder: recorder,
		services: services,
		pods:     pods,
	}
	p.ctx, p.cancel = context.WithCancel(ctx)
	p.start()
	return p
}

func (p *poller) update(ssr *v1.ServiceScaleRecommendation) {
	if ssr == nil {
		return
	}

	p.ssrLock.Lock()
	defer p.ssrLock.Unlock()
	p.ssr = ssr
}

func (p *poller) start() {
	for range ticker.Context(p.ctx, metrics.DefaultConfig.TickInterval) {
		p.ssrLock.Lock()
		ssr := p.ssr.DeepCopy()
		p.ssrLock.Unlock()

		if ssr.Spec.PrometheusURL == "" {
			continue
		}

		if err := p.getStats(ssr); err != nil {
			logrus.Errorf("Failed to get stats for %s: %v", ssr.Spec.ServiceNameToRead)
		}
	}
}

func (p *poller) loadClient(ssr *v1.ServiceScaleRecommendation) error {
	if p.promURL == ssr.Spec.PrometheusURL {
		return nil
	}

	apiClient, err := api.NewClient(api.Config{
		Address: ssr.Spec.PrometheusURL,
	})
	if err != nil {
		return nil
	}

	p.promAPI = promv1.NewAPI(apiClient)
	p.promURL = ssr.Spec.PrometheusURL

	return nil
}

func (p *poller) getStats(ssr *v1.ServiceScaleRecommendation) error {
	if err := p.loadClient(ssr); err != nil {
		return err
	}

	svc, err := p.services.Get(ssr.Namespace, ssr.Spec.ServiceNameToRead)
	if err != nil {
		return err
	}

	selector := labels.SelectorFromSet(labels.Set(svc.Spec.Selector))

	pods, err := p.pods.List(ssr.Namespace, selector)
	if err != nil {
		return err
	}

	for _, pod := range pods {
		stat, err := p.getPodStat(pod)
		if err != nil {
			return err
		}
		p.recorder(stat)
	}

	return nil
}

func (p *poller) getPodStat(pod *corev1.Pod) (autoscaler.Stat, error) {
	t := time.Now()
	stat := autoscaler.Stat{
		PodName:                   pod.Name,
		Time:                      &t,
		AverageConcurrentRequests: 100,
		RequestCount:              1,
	}

	return stat, nil
}

func (p *poller) Stop() {
	p.cancel()
}
