package servicescale

import (
	"bufio"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	riov1 "github.com/rancher/rio/pkg/apis/rio.cattle.io/v1"
	riov1controller "github.com/rancher/rio/pkg/generated/controllers/rio.cattle.io/v1"
	services2 "github.com/rancher/rio/pkg/services"
	corev1controller "github.com/rancher/wrangler-api/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

type SimpleScale struct {
	namespace   string
	serviceName string
	app         string
	version     string
	stop        chan struct{}
	stopScaling chan struct{}
	httpClient  *http.Client
	metrics     metrics
	podLister   corev1controller.PodCache
	services    riov1controller.ServiceController

	lastUpdatedScale int
}

const (
	houseKeepTime    = time.Minute * 5
	houseKeepTicker  = time.Minute * 5
	tickerInterval   = time.Second * 5
	decisionInterval = time.Second * 15
)

func NewSimpleScale(svc *riov1.Service, podCache corev1controller.PodCache, services riov1controller.ServiceController) SimpleScale {
	app, version := services2.AppAndVersion(svc)
	return SimpleScale{
		namespace:   svc.Namespace,
		serviceName: svc.Name,
		app:         app,
		version:     version,
		stop:        make(chan struct{}),
		stopScaling: make(chan struct{}),
		httpClient:  http.DefaultClient,
		metrics: metrics{
			stop: make(chan struct{}),
			lock: sync.RWMutex{},
		},
		podLister: podCache,
		services:  services,
	}
}

type metrics struct {
	stop  chan struct{}
	lock  sync.RWMutex
	stats []metric
}

type metric struct {
	time          time.Time
	activeRequest int
	readyPods     int
}

func (s *metrics) clean(offset int) {
	s.lock.Lock()
	defer s.lock.Unlock()
	logrus.Debugf("cleaning up %v stats", offset)
	s.stats = s.stats[offset:len(s.stats)]
}

func (s *metrics) read(index int) metric {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.stats[index]
}

func (s *metrics) append(m metric) {
	s.lock.Lock()
	defer s.lock.Unlock()
}

func (s *metrics) houseKeeping() {
	ticker := time.Tick(houseKeepTicker)
	for {
		select {
		case <-ticker:
			offset := 0
			for i, stat := range s.stats {
				if time.Now().Before(stat.time.Add(houseKeepTime)) {
					offset = i
					break
				}
			}
			s.clean(offset)
		case <-s.stop:
			logrus.Debugf("stop housekeeping thread")
			return
		}
	}

}

func (s *SimpleScale) Scale() error {
	s.metrics.lock.RLock()
	defer s.metrics.lock.RUnlock()
	var total, count, readyPodTotal int
	for i := len(s.metrics.stats) - 1; i >= len(s.metrics.stats)-12; i-- {
		if i < 0 {
			break
		}
		total += s.metrics.stats[i].activeRequest
		count++
		readyPodTotal += s.metrics.stats[i].readyPods
	}

	svc, err := s.services.Cache().Get(s.namespace, s.serviceName)
	if err != nil {
		return err
	}

	/*
		Desired rate is calculated by in-flight requests divided by concurrency.
		Scale is calculated by desired rate multiplied by desired rate.
		For example, if current replica is 2, in-flight requests per pod is 30 and concurrency is 10,
		The desired scale should be 2 * 30 / 10 = 6
	*/

	var desiredScale int32

	if count != 0 {
		currentReplica := float64(readyPodTotal) / float64(count)
		if currentReplica == 0 {
			currentReplica = 1
		}

		var rate float64
		concurrency := svc.Spec.Autoscale.Concurrency
		if concurrency == 0 {
			rate = 1
		} else {
			rate = (float64(total) / float64(count)) / float64(concurrency)
		}

		desiredScale = int32(math.Ceil(currentReplica * rate))
		logrus.Debugf("average ready pods: %v, scale rate: %v, desired scale: %v", currentReplica, rate, desiredScale)
	}

	shouldScale := int(bounded(desiredScale, *svc.Spec.Autoscale.MinReplicas, *svc.Spec.Autoscale.MaxReplicas))
	if svc.Status.ComputedReplicas != nil && *svc.Status.ComputedReplicas == shouldScale {
		return nil
	}

	logrus.Debugf("Updating service to scale %v", shouldScale)

	// prevent scaling down too frequently
	if shouldScale-s.lastUpdatedScale < 0 {
		scaleDownRate := s.lastUpdatedScale - shouldScale
		threshold := int(math.Ceil(float64(s.lastUpdatedScale) / 2.0))
		if scaleDownRate < threshold {
			logrus.Debugf("scaling down rate %v is less than %v, do no work", scaleDownRate, threshold)
			return nil
		}
	}

	svc.Status.ComputedReplicas = &shouldScale
	if _, err = s.services.UpdateStatus(svc); err != nil {
		return err
	}
	s.lastUpdatedScale = shouldScale
	return nil
}

func (s *SimpleScale) Start() {
	go s.metrics.houseKeeping()

	go func() {
		ticker := time.Tick(tickerInterval)
		for {
			select {
			case <-ticker:
				if err := s.scrape(); err != nil {
					logrus.Warnf("Failed to scrape metric for %s/%s, error: %v", s.namespace, s.serviceName, err)
				}
			case <-s.stop:
				logrus.Debugf("Stop watching metric for %s/%s", s.namespace, s.serviceName)
				return
			}
		}
	}()

	go func() {
		ticker := time.Tick(decisionInterval)
		for {
			select {
			case <-ticker:
				if err := s.Scale(); err != nil {
					logrus.Warnf("Failed to scale for %s/%s, error: %v", s.namespace, s.serviceName, err)
				}
			case <-s.stopScaling:
				logrus.Debugf("Stop autoscaling for %s/%s", s.namespace, s.serviceName)
				return
			}
		}
	}()

}

func (s *SimpleScale) Stop() {
	s.stop <- struct{}{}
	s.stopScaling <- struct{}{}
	s.metrics.stop <- struct{}{}
}

func (s *SimpleScale) ReportMetric() {
	s.metrics.lock.Lock()
	defer s.metrics.lock.Unlock()

	logrus.Debugf("reporting traffic manually for %s/%s", s.namespace, s.app)
	s.metrics.stats = append(s.metrics.stats, metric{
		time:          time.Now(),
		activeRequest: 1,
		readyPods:     1,
	})
}

func (s *SimpleScale) scrape() error {
	r1, err := labels.NewRequirement("app", selection.Equals, []string{s.app})
	if err != nil {
		return err
	}
	r2, err := labels.NewRequirement("version", selection.Equals, []string{s.version})
	if err != nil {
		return err
	}
	selector := labels.NewSelector().Add(*r1, *r2)
	pods, err := s.podLister.List(s.namespace, selector)
	if err != nil {
		return err
	}

	var totalActiveRequest, inboundActiveRequests, outbountActiveRequests, readyPods int
	stat := metric{
		time: time.Now(),
	}
	podMap := map[string]*corev1.Pod{}
	for i := range pods {
		if pods[i].Status.Phase == corev1.PodRunning {
			podMap[pods[i].Name] = pods[i]
		}
	}

	for _, pod := range podMap {
		metricURL := fmt.Sprintf("http://%s:4191/metrics", pod.Status.PodIP)
		responseTotalMatchCriteria := fmt.Sprintf("response_total{authority=\"%s-%s.%s.svc.cluster.local", s.app, s.version, s.namespace)
		requestTotalMatchCriteria := fmt.Sprintf("request_total{authority=\"%s-%s.%s.svc.cluster.local", s.app, s.version, s.namespace)

		req, err := http.NewRequest(http.MethodGet, metricURL, nil)
		if err != nil {
			return err
		}
		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil
		}
		defer resp.Body.Close()
		scannner := bufio.NewScanner(resp.Body)

		inboundActiveRequests = calculateActiveRequests(scannner, requestTotalMatchCriteria, responseTotalMatchCriteria, true)
		outbountActiveRequests = calculateActiveRequests(scannner, requestTotalMatchCriteria, responseTotalMatchCriteria, false)
		totalActiveRequest = inboundActiveRequests + outbountActiveRequests
		readyPods++
	}

	if readyPods == 0 {
		stat.activeRequest = 0
	} else {
		stat.activeRequest = int(float64(totalActiveRequest) / float64(readyPods))
	}
	stat.readyPods = readyPods

	logrus.Debugf("collect metric for %s/%s, total request: %v(inbound %v, outbound %v), average in-flight request per pod: %v, ready pod: %v", s.namespace, s.serviceName, totalActiveRequest, inboundActiveRequests, outbountActiveRequests, stat.activeRequest, readyPods)
	s.metrics.stats = append(s.metrics.stats, stat)
	return nil
}

func calculateActiveRequests(scanner *bufio.Scanner, requestMatch, responseMatch string, inbound bool) int {
	direction := "direction=\"inbound\""
	if !inbound {
		direction = "direction=\"outbound\""
	}
	var request, response int
	for scanner.Scan() {
		response = match(scanner, responseMatch, direction)
		request = match(scanner, requestMatch, direction)
	}
	return request - response
}

func match(scanner *bufio.Scanner, m, direction string) int {
	var result int
	if strings.Contains(scanner.Text(), m) && strings.Contains(scanner.Text(), direction) {
		parts := strings.Split(scanner.Text(), " ")
		if len(parts) == 2 {
			rqs, _ := strconv.Atoi(parts[1])
			result = rqs
		}
	}
	return result
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
