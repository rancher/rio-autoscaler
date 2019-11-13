package servicescale

import (
	"context"
	"sync"

	riov1 "github.com/rancher/rio/pkg/apis/rio.cattle.io/v1"
	riov1controller "github.com/rancher/rio/pkg/generated/controllers/rio.cattle.io/v1"
	corev1controller "github.com/rancher/wrangler-api/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
)

type SSRHandler struct {
	ctx         context.Context
	autoscalers map[string]*SimpleScale
	lock        *sync.RWMutex
	pods        corev1controller.PodCache
	services    riov1controller.ServiceController
}

func NewHandler(ctx context.Context,
	services riov1controller.ServiceController,
	podClientCache corev1controller.PodCache,
	autoscalers map[string]*SimpleScale,
	lock *sync.RWMutex) *SSRHandler {

	return &SSRHandler{
		ctx:         ctx,
		services:    services,
		pods:        podClientCache,
		lock:        lock,
		autoscalers: autoscalers,
	}
}

func (s *SSRHandler) OnChange(key string, svc *riov1.Service) (*riov1.Service, error) {
	if svc == nil || svc.DeletionTimestamp != nil {
		if ss, ok := s.autoscalers[key]; ok {
			ss.Stop()
			s.lock.Lock()
			defer s.lock.Unlock()
			logrus.Debugf("deleting autoscale key %v", key)
			delete(s.autoscalers, key)
		}
		return nil, nil
	}

	if !autoscaleEnabled(svc) {
		return nil, nil
	}

	if _, ok := s.autoscalers[key]; !ok {
		ss := NewSimpleScale(svc, s.pods, s.services)
		ss.Start()
		s.lock.Lock()
		defer s.lock.Unlock()
		logrus.Debugf("adding autoscaler key %v", key)
		s.autoscalers[key] = &ss
	}
	return svc, nil
}

func autoscaleEnabled(service *riov1.Service) bool {
	return service.Spec.Autoscale != nil && service.Spec.Autoscale.MinReplicas != nil && service.Spec.Autoscale.MaxReplicas != nil && *service.Spec.Autoscale.MinReplicas != *service.Spec.Autoscale.MaxReplicas
}
