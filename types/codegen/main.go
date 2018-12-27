package main

import (
	"github.com/rancher/norman/generator"
	"github.com/rancher/rio-autoscaler/types/apis/rio-autoscale.cattle.io/v1"
	"github.com/sirupsen/logrus"
	"k8s.io/api/apps/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

const (
	pkg = "github.com/rancher/rio-autoscaler/types"
)

func main() {
	if err := generator.DefaultGenerate(v1.Schemas, pkg, false, nil); err != nil {
		logrus.Fatal(err)
	}

	if err := generator.ControllersForForeignTypes(pkg, v1beta1.SchemeGroupVersion,
		[]interface{}{
			v1beta1.Deployment{},
		}, nil); err != nil {
		logrus.Fatal(err)
	}

	if err := generator.ControllersForForeignTypes(pkg, corev1.SchemeGroupVersion,
		[]interface{}{
			corev1.Service{},
			corev1.Pod{},
			corev1.Endpoints{},
		}, nil); err != nil {
		logrus.Fatal(err)
	}
}
