package v1

import (
	"time"

	"github.com/rancher/norman/types"
	"github.com/rancher/norman/types/factory"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	APIVersion = types.APIVersion{
		Group:   "rio-autoscale.cattle.io",
		Version: "v1",
		Path:    "/v1-rio-autoscale",
	}
	Schemas = factory.
		Schemas(&APIVersion).
		MustImport(&APIVersion, ServiceScaleRecommendation{})
)

type ServiceScaleRecommendation struct {
	types.Namespaced

	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceScaleRecommendationSpec   `json:"spec,omitempty"`
	Status ServiceScaleRecommendationStatus `json:"status,omitempty"`
}

type ServiceScaleRecommendationSpec struct {
	ServiceNameToRead   string `json:"serviceNameToRead,omitempty"`
	ServiceNameToChange string `json:"serviceNameToChange,omitempty"`
	ZeroScaleService    string `json:"zeroScaleService,omitempty"`
	DeploymentName      string `json:"deploymentName,omitempty"`
	MinScale            int32  `json:"minScale,omitempty"`
	MaxScale            int32  `json:"maxScale,omitempty"`
	Concurrency         int    `json:"concurrency,omitempty"`
	PrometheusURL       string `json:"prometheusURL,omitempty"`
	AlgorithmConfig     string `json:"algorithmConfig,omitempty"`
}

type ServiceScaleRecommendationStatus struct {
	DesiredScale *int32 `json:"desiredScale,omitempty"`
}

type AlgorithmConfig struct {
	MaxScaleUpRate float64       `json:"maxScaleUpRate,omitempty"`
	StableWindow   time.Duration `json:"stableWindow,omitempty"`
	PanicWindow    time.Duration `json:"panicWindow,omitempty"`
	TickInterval   time.Duration `json:"tickInterval,omitempty"`
}
