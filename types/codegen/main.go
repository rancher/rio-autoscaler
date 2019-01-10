package main

import (
	"github.com/rancher/norman/generator"
	"github.com/sirupsen/logrus"
	"k8s.io/api/apps/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

const (
	pkg = "github.com/rancher/rio-autoscaler/types"
)

func main() {
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
			corev1.ConfigMap{},
		}, nil); err != nil {
		logrus.Fatal(err)
	}
}
