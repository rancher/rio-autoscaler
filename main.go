//go:generate go run types/codegen/cleanup/main.go
//go:generate go run types/codegen/main.go

package main

import (
	"context"
	"os"

	"github.com/rancher/norman"
	"github.com/rancher/norman/pkg/resolvehome"
	"github.com/rancher/norman/signal"
	"github.com/rancher/rio-autoscaler/pkg/server"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

var (
	VERSION = "v0.0.0-dev"
)

func main() {
	app := cli.NewApp()
	app.Name = "rio-autoscaler"
	app.Version = VERSION
	app.Usage = "rio-autoscaler needs help!"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "kubeconfig",
			EnvVar: "KUBECONFIG",
			Value:  "${HOME}/.kube/config",
		},
	}
	app.Action = run

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func run(c *cli.Context) error {
	logrus.Info("Starting controller")
	ctx := signal.SigTermCancelContext(context.Background())

	kubeConfig, err := resolvehome.Resolve(c.String("kubeconfig"))
	if err != nil {
		return err
	}

	ctx, _, err = server.Config().Build(ctx, &norman.Options{
		K8sMode:    "external",
		KubeConfig: kubeConfig,
	})

	if err != nil {
		return err
	}
	<-ctx.Done()
	return nil
}
