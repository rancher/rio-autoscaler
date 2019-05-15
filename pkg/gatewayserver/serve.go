package gatewayserver

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	activatorutil "github.com/knative/serving/pkg/activator/util"
	"github.com/rancher/rio-autoscaler/pkg/controllers/gateway"
	"github.com/rancher/rio-autoscaler/pkg/logger"
	"github.com/rancher/rio-autoscaler/types"
	riov1controller "github.com/rancher/rio/pkg/generated/controllers/rio.cattle.io/v1"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/proxy"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	maxRetries             = 18 // the sum of all retries would add up to 1 minute
	minRetryInterval       = 100 * time.Millisecond
	exponentialBackoffBase = 1.3
	RioNameHeader          = "X-Rio-ServiceName"
	RioNamespaceHeader     = "X-Rio-Namespace"
	RioPortHeader          = "X-Rio-ServicePort"
)

func NewHandler(rContext *types.Context) Handler {
	return Handler{
		services: rContext.Rio.Rio().V1().Service(),
	}
}

type Handler struct {
	services riov1controller.ServiceController
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := r.Header.Get(RioNameHeader)
	namespace := r.Header.Get(RioNamespaceHeader)
	port := r.Header.Get(RioPortHeader)

	rioSvc, err := h.services.Get(namespace, name, metav1.GetOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	if rioSvc.Status.ObservedScale != nil && *rioSvc.Status.ObservedScale == 0 {
		rioSvc.Status.ObservedScale = &[]int{1}[0]
		t := metav1.NewTime(time.Now())
		rioSvc.Status.ScaleFromZeroTimestamp = &t
		logrus.Infof("Activating service %s to scale 1", rioSvc.Name)
		if _, err := h.services.Update(rioSvc); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
	}

	timer := time.After(time.Minute)

	endpointCh, endpointNotReady := gateway.EndpointChanMap.Load(fmt.Sprintf("%s.%s", name, namespace))
	if !endpointNotReady {
		serveFQDN(name, namespace, port, w, r)
		return
	}
	select {
	case <-timer:
		http.Error(w, "timeout waiting for endpoint to be active", http.StatusGatewayTimeout)
		return
	case _, ok := <-endpointCh.(chan struct{}):
		if !ok {
			serveFQDN(name, namespace, port, w, r)
			return
		}
	}
}

func serveFQDN(name, namespace, port string, w http.ResponseWriter, r *http.Request) {
	targetURL := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s.%s.svc.cluster.local:%s", name, namespace, port),
		Path:   r.URL.Path,
	}
	r.URL = targetURL
	r.URL.Host = targetURL.Host
	r.Host = targetURL.Host

	// todo: check if 503 is actually coming from application or envoy
	shouldRetry := activatorutil.RetryStatus(http.StatusServiceUnavailable)
	backoffSettings := wait.Backoff{
		Duration: minRetryInterval,
		Factor:   exponentialBackoffBase,
		Steps:    maxRetries,
	}

	rt := activatorutil.NewRetryRoundTripper(activatorutil.AutoTransport, logger.SugaredLogger, backoffSettings, shouldRetry)
	httpProxy := proxy.NewUpgradeAwareHandler(targetURL, rt, true, false, er)
	httpProxy.ServeHTTP(w, r)
}

var er = &errorResponder{}

type errorResponder struct {
}

func (e *errorResponder) Error(w http.ResponseWriter, req *http.Request, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	if _, err := w.Write([]byte(err.Error())); err != nil {
		logrus.Errorf("error writing response: %v", err)
	}
}
