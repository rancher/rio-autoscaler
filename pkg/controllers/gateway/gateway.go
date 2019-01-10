package gateway

import (
	"context"
	"fmt"
	"sync"

	"github.com/rancher/rio-autoscaler/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	EndpointChanMap = sync.Map{}
)

func Register(ctx context.Context, rContext *types.Context) error {
	e := endpointHandler{}
	rContext.Core.Endpoints.OnChange(ctx, "gateway-endpoint-watcher", e.Sync)
	rContext.Core.Endpoints.OnRemove(ctx, "gateway-endpoint-watcher", e.Remove)

	return nil
}

type endpointHandler struct{}

func (e endpointHandler) Sync(obj *corev1.Endpoints) (runtime.Object, error) {
	// todo: add a filter only for scale-to-zero services so that we don't have to keep a channel for every endpoint
	if obj != nil && obj.DeletionTimestamp == nil {
		ch := make(chan struct{}, 0)
		EndpointChanMap.LoadOrStore(fmt.Sprintf("%s.%s", obj.Name, obj.Namespace), ch)
		if isEndpointReady(obj) {
			o, ok := EndpointChanMap.Load(fmt.Sprintf("%s.%s", obj.Name, obj.Namespace))
			if ok {
				c := o.(chan struct{})
				close(c)
				EndpointChanMap.Delete(fmt.Sprintf("%s.%s", obj.Name, obj.Namespace))
			}
		}
	}
	return obj, nil
}

func isEndpointReady(obj *corev1.Endpoints) bool {
	ready := true
	if len(obj.Subsets) == 0 {
		ready = false
	}
	for _, subnet := range obj.Subsets {
		if len(subnet.NotReadyAddresses) > 0 {
			ready = false
		}
	}
	return ready
}

func (e endpointHandler) Remove(obj *corev1.Endpoints) (runtime.Object, error) {
	if obj != nil {
		EndpointChanMap.Delete(fmt.Sprintf("%s.%s", obj.Name, obj.Namespace))
	}
	return obj, nil
}
