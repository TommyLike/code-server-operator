/*
Copyright 2019 tommylikehu@gmail.com.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	csv1alpha1 "github.com/tommylike/code-server-operator/api/v1alpha1"
	"github.com/tommylike/code-server-operator/controllers"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

const (
	REQUEST_CHAN_SIZE = 10
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = csv1alpha1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

var onlyOneSignalHandler = make(chan struct{})
var shutdownSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}

func SetupSignalHandler() (stopCh <-chan struct{}) {
	close(onlyOneSignalHandler) // panics when called twice

	stop := make(chan struct{})
	c := make(chan os.Signal, 2)
	signal.Notify(c, shutdownSignals...)
	go func() {
		<-c
		close(stop)
		<-c
		os.Exit(1) // second signal. Exit directly.
	}()

	return stop
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	csOption := controllers.CodeServerOption{}
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&csOption.DomainName, "domain-name", "code.tommylike.me", "Code server domain name.")
	flag.StringVar(&csOption.ExporterImage, "default-exporter", "tommylike/active-exporter-x86:stable",
		"Default exporter image used as a code server sidecar.")
	flag.IntVar(&csOption.ProbeInterval, "probe-interval", 20,
		"time in seconds between two probes on code server instance.")
	flag.IntVar(&csOption.MaxProbeRetry, "max-probe-retry", 4,
		"count before marking code server inactive when failed to probe liveness")
	flag.StringVar(&csOption.HttpsSecretName, "secrets-name", "web-secrets", "Secret which holds the https cert(tls.cert) and key file(tls.key).")
	flag.Parse()

	ctrl.SetLogger(zap.New(func(o *zap.Options) {
		o.Development = true
	}))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		LeaderElection:     enableLeaderElection,
		Port:               9443,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}
	csRequest := make(chan controllers.CodeServerRequest, REQUEST_CHAN_SIZE)
	if err = (&controllers.CodeServerReconciler{
		Client:  mgr.GetClient(),
		Log:     ctrl.Log.WithName("controllers").WithName("CodeServer"),
		Scheme:  mgr.GetScheme(),
		Options: &csOption,
		ReqCh:   csRequest,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CodeServer")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder
	probeTicker := time.NewTicker(time.Duration(csOption.ProbeInterval) * time.Second)
	defer probeTicker.Stop()
	//setup code server watcher
	codeServerWatcher := controllers.NewCodeServerWatcher(
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName("CodeServerWatcher"),
		mgr.GetScheme(),
		&csOption,
		csRequest,
		probeTicker.C)
	stopCh := ctrl.SetupSignalHandler()
	go codeServerWatcher.Run(stopCh)

	setupLog.Info("starting manager")
	if err := mgr.Start(stopCh); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
