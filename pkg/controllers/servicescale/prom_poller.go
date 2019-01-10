package servicescale

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/common/model"

	"github.com/knative/serving/pkg/autoscaler"
	"github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/rancher/rancher/pkg/ticker"
	"github.com/rancher/rio-autoscaler/pkg/metrics"
	corev1client "github.com/rancher/rio-autoscaler/types/apis/core/v1"
	"github.com/rancher/rio/types/apis/rio-autoscale.cattle.io/v1"
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
	go p.start()
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
			logrus.Errorf("Failed to get stats for %s: %v", ssr.Spec.ServiceNameToRead, err)
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

	// QPS
	rc, err := p.promAPI.Query(context.Background(), formatQPSQuery(pod), t)
	if err != nil {
		return autoscaler.Stat{}, err
	}
	if rc.Type() != model.ValVector {
		return autoscaler.Stat{}, errors.Errorf("unexpected value type %v", rc.Type().String())
	}
	qps := 0.0
	vector := rc.(model.Vector)
	for _, s := range vector {
		qps = float64(s.Value)
	}

	// active connection
	ac, err := p.promAPI.Query(context.Background(), formatActiveRequest(pod), t)
	if ac.Type() != model.ValVector {
		return autoscaler.Stat{}, errors.Errorf("unexpected value type %v", ac.Type().String())
	}
	acs := 0.0
	vector = ac.(model.Vector)
	for _, s := range vector {
		acs = float64(s.Value)
	}

	stat := autoscaler.Stat{
		PodName:                   pod.Name,
		Time:                      &t,
		AverageConcurrentRequests: acs,
		RequestCount:              int32(qps),
	}

	return stat, nil
}

func (p *poller) Stop() {
	p.cancel()
}

func formatQPSQuery(pod *corev1.Pod) string {
	return fmt.Sprintf("rate(envoy_http_downstream_rq_total{pod_name=\"%s\",namespace=\"%s\",http_conn_manager_prefix=~\"%s.*\"}[1m])", pod.Name, pod.Namespace, pod.Status.PodIP)
}

func formatActiveRequest(pod *corev1.Pod) string {
	return fmt.Sprintf("envoy_http_downstream_rq_active{pod_name=\"%s\",namespace=\"%s\",http_conn_manager_prefix=~\"%s.*\"}", pod.Name, pod.Namespace, pod.Status.PodIP)
}
