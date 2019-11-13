package gatewayserver

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/rancher/rio/modules/service/controllers/service/populate/serviceports"

	"github.com/rancher/rio-autoscaler/pkg/controllers/servicescale"
	"github.com/rancher/rio-autoscaler/types"
	riov1controller "github.com/rancher/rio/pkg/generated/controllers/rio.cattle.io/v1"
	"github.com/rancher/rio/pkg/services"
	name2 "github.com/rancher/wrangler/pkg/name"
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
)

func NewHandler(rContext *types.Context, lock *sync.RWMutex, autoscalers map[string]*servicescale.SimpleScale) Handler {
	return Handler{
		services:    rContext.Rio.Rio().V1().Service(),
		lock:        lock,
		autoscalers: autoscalers,
	}
}

type Handler struct {
	services    riov1controller.ServiceController
	autoscalers map[string]*servicescale.SimpleScale
	lock        *sync.RWMutex
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	name := r.Header.Get(RioNameHeader)
	namespace := r.Header.Get(RioNamespaceHeader)

	svc, err := h.services.Get(namespace, name, metav1.GetOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	svc.Status.ComputedReplicas = &[]int{1}[0]

	h.lock.Lock()
	sc, ok := h.autoscalers[fmt.Sprintf("%s/%s", namespace, name)]
	if ok {
		sc.ReportMetric()
	}
	h.lock.Unlock()

	logrus.Infof("Activating service %s to scale 1", svc.Name)
	if _, err := h.services.UpdateStatus(svc); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	checkPort := ""
	for _, port := range serviceports.ContainerPorts(svc) {
		if port.IsExposed() && port.IsHTTP() {
			checkPort = strconv.Itoa(int(port.Port))
			continue
		}
	}

	app, version := services.AppAndVersion(svc)
	serveFQDN(name2.SafeConcatName(app, version), namespace, checkPort, w, r)

	logrus.Infof("activating service %s/%s takes %v seconds", svc.Name, svc.Namespace, time.Now().Sub(start).Seconds())
	return
}

func serveFQDN(name, namespace, port string, w http.ResponseWriter, r *http.Request) {
	targetURL := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s.%s.svc:%s", name, namespace, port),
		Path:   r.URL.Path,
	}
	r.URL = targetURL
	r.URL.Host = targetURL.Host
	r.Host = targetURL.Host

	shouldRetry := []retryCond{retryStatus(http.StatusServiceUnavailable), retryStatus(http.StatusBadGateway)}
	backoffSettings := wait.Backoff{
		Duration: minRetryInterval,
		Factor:   exponentialBackoffBase,
		Steps:    maxRetries,
	}

	rt := newRetryRoundTripper(autoTransport, backoffSettings, shouldRetry...)
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
