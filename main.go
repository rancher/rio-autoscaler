//go:generate go run types/codegen/cleanup/main.go
//go:generate go run types/codegen/main.go

package main

import (
	"context"
	"net/http"
	"os"

	"github.com/rancher/rio-autoscaler/pkg/controllers"
	"k8s.io/apimachinery/pkg/util/runtime"
	"github.com/rancher/rio-autoscaler/pkg/gatewayserver"
	"github.com/rancher/rio-autoscaler/pkg/logger"
	"github.com/rancher/rio-autoscaler/types"
	"github.com/rancher/wrangler/pkg/leader"
	"github.com/rancher/wrangler/pkg/signals"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"k8s.io/client-go/tools/clientcmd"
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
		},
		cli.StringFlag{
			Name: "debug",
			EnvVar: "DEBUG",
		},
	}
	app.Commands = []cli.Command{
		{
			Name:   "gateway",
			Action: runGateway,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "listen",
					Value: "8080",
				},
			},
			Usage: "Run autoscaler gateway",
		},
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func runGateway(c *cli.Context) error {
	logrus.Info("Starting controller")
	ctx := signals.SetupSignalHandler(context.Background())

	if err := logger.InitLogger(c.GlobalString("debug")); err != nil {
		return err
	}

	kubeconfig := c.String("kubeconfig")
	namespace := os.Getenv("NAMESPACE")

	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return err
	}

	ctx, rioContext := types.BuildContext(ctx, namespace, restConfig)
	go func() {
		leader.RunOrDie(ctx, namespace, "rio-autoscaler", rioContext.K8s, func(ctx context.Context) {
			runtime.Must(controllers.Register(ctx, rioContext))
			runtime.Must(rioContext.Start(ctx))
			<-ctx.Done()
		})
	}()

	gatewayHandler := gatewayserver.NewHandler(rioContext)
	srv := &http.Server{
		Addr:    ":" + c.String("listen"),
		Handler: h2c.NewHandler(gatewayHandler, &http2.Server{}),
	}

	go func() {
		logrus.Infof("starting gateway server on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil {
			logrus.Errorf("Error running HTTP server: %v", err)
		}
	}()

	<-ctx.Done()
	return srv.Shutdown(ctx)
}
