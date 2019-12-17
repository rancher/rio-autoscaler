//go:generate go run types/codegen/cleanup/main.go
//go:generate go run types/codegen/main.go

package main

import (
	"context"
	"net/http"
	"os"
	"sync"

	"github.com/rancher/rio-autoscaler/pkg/controllers"
	"github.com/rancher/rio-autoscaler/pkg/controllers/servicescale"
	"github.com/rancher/rio-autoscaler/pkg/gatewayserver"
	"github.com/rancher/rio-autoscaler/types"
	"github.com/rancher/wrangler/pkg/leader"
	"github.com/rancher/wrangler/pkg/signals"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"k8s.io/apimachinery/pkg/util/runtime"
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
			Name:  "srv-addr",
			Usage: "Address to server on",
			Value: ":80",
		},
		cli.BoolFlag{
			Name: "debug",
		},
	}
	app.Action = run

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func run(c *cli.Context) error {
	logrus.Info("Starting controller")
	if c.Bool("debug") {
		logrus.SetLevel(logrus.DebugLevel)
	}

	ctx := signals.SetupSignalHandler(context.Background())

	kubeconfig := c.String("kubeconfig")
	namespace := os.Getenv("NAMESPACE")

	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return err
	}

	lock := &sync.RWMutex{}
	autoscalers := map[string]*servicescale.SimpleScale{}

	ctx, rioContext := types.BuildContext(ctx, namespace, restConfig)
	go func() {
		leader.RunOrDie(ctx, namespace, "rio-autoscaler", rioContext.K8s, func(ctx context.Context) {
			runtime.Must(controllers.Register(ctx, rioContext, lock, autoscalers))
			runtime.Must(rioContext.Start(ctx))
			<-ctx.Done()
		})
	}()

	gatewayHandler := gatewayserver.NewHandler(rioContext, lock, autoscalers)
	srv := &http.Server{
		Addr:    c.String("srv-addr"),
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
