/*
Copyright 2024.

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
	"crypto/tls"
	"flag"
	"os"
	"runtime/debug"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.uber.org/zap/zapcore"

	"github.com/argoproj-labs/gitops-promoter/internal/settings"
	"github.com/argoproj-labs/gitops-promoter/internal/types/argocd"
	"github.com/argoproj-labs/gitops-promoter/internal/utils/gitpaths"
	"github.com/argoproj-labs/gitops-promoter/internal/webhookreceiver"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	promoterv1alpha1 "github.com/argoproj-labs/gitops-promoter/api/v1alpha1"
	"github.com/argoproj-labs/gitops-promoter/internal/controller"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(promoterv1alpha1.AddToScheme(scheme))
	utilruntime.Must(argocd.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func newControllerCommand(clientConfig clientcmd.ClientConfig) *cobra.Command {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var pprofAddr string

	cmd := &cobra.Command{
		Use:   "controller",
		Short: "GitOps Promoter controller",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runController(
				metricsAddr,
				probeAddr,
				pprofAddr,
				enableLeaderElection,
				secureMetrics,
				enableHTTP2,
				clientConfig,
			)
		},
	}

	cmd.Flags().StringVar(&metricsAddr, "metrics-bind-address", ":9080", "The address the metric endpoint binds to.")
	cmd.Flags().StringVar(&probeAddr, "health-probe-bind-address", ":9081", "The address the probe endpoint binds to.")
	cmd.Flags().StringVar(&pprofAddr, "pprof-bind-address", "",
		"The address the pprof endpoint binds to. If unset, pprof is disabled.")
	cmd.Flags().BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	cmd.Flags().BoolVar(&secureMetrics, "metrics-secure", false, "If set the metrics endpoint is served securely")
	cmd.Flags().BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")

	return cmd
}

func runController(
	metricsAddr string,
	probeAddr string,
	pprofAddr string,
	enableLeaderElection bool,
	secureMetrics bool,
	enableHTTP2 bool,
	clientConfig clientcmd.ClientConfig,
) error {
	controllerNamespace, _, err := clientConfig.Namespace()
	if err != nil {
		setupLog.Error(err, "failed to get namespace")
		os.Exit(1)
	}

	// Recover any panic and log using the configured logger. This ensures that panics get logged in JSON format if
	// JSON logging is enabled.
	defer func() {
		if r := recover(); r != nil {
			setupLog.Error(nil, "recovered from panic", "panic", r, "trace", string(debug.Stack()))
			os.Exit(1)
		}
	}()

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	tlsOpts := []func(*tls.Config){}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress:   metricsAddr,
			SecureServing: secureMetrics,
			TLSOpts:       tlsOpts,
		},
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		PprofBindAddress:       pprofAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "b21a50c7.argoproj.io",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil || mgr == nil {
		panic("unable to start manager")
	}

	settingsMgr := settings.NewManager(mgr.GetClient(), settings.ManagerConfig{
		ControllerNamespace: controllerNamespace,
	})

	if err = (&controller.PullRequestReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("PullRequest"),
		SettingsMgr: settingsMgr,
	}).SetupWithManager(mgr); err != nil {
		panic("unable to create PullRequest controller")
	}
	if err = (&controller.CommitStatusReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("CommitStatus"),
		SettingsMgr: settingsMgr,
	}).SetupWithManager(mgr); err != nil {
		panic("unable to create CommitStatus controller")
	}
	if err = (&controller.RevertCommitReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("RevertCommit"),
	}).SetupWithManager(mgr); err != nil {
		panic("unable to create RevertCommit controller")
	}

	if err = (&controller.PromotionStrategyReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("PromotionStrategy"),
		SettingsMgr: settingsMgr,
	}).SetupWithManager(mgr); err != nil {
		panic("unable to create PromotionStrategy controller")
	}
	if err = (&controller.ScmProviderReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("ScmProvider"),
	}).SetupWithManager(mgr); err != nil {
		panic("unable to create ScmProvider controller")
	}
	if err = (&controller.GitRepositoryReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		panic("unable to create GitRepository controller")
	}
	if err = (&controller.ChangeTransferPolicyReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("ChangeTransferPolicy"),
		SettingsMgr: settingsMgr,
	}).SetupWithManager(mgr); err != nil {
		panic("unable to create ChangeTransferPolicy controller")
	}
	if err = (&controller.ArgoCDCommitStatusReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		Recorder:    mgr.GetEventRecorderFor("ArgoCDCommitStatus"),
		SettingsMgr: settingsMgr,
	}).SetupWithManager(mgr); err != nil {
		panic("unable to create ArgoCDCommitStatus controller")
	}
	if err = (&controller.ControllerConfigurationReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		panic("unable to create ControllerConfiguration controller")
	}
	if err = (&controller.ClusterScmProviderReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		panic("unable to create ClusterScmProvider controller")
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		panic("unable to set up health check")
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		panic("unable to set up ready check")
	}

	processSignals := ctrl.SetupSignalHandler()

	whr := webhookreceiver.NewWebhookReceiver(mgr)
	go func() {
		err = whr.Start(processSignals, ":3333")
		if err != nil {
			setupLog.Error(err, "unable to start webhook receiver")
			err = syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
			if err != nil {
				setupLog.Error(err, "unable to kill process")
			}
		}
	}()

	setupLog.Info("starting manager")
	if err := mgr.Start(processSignals); err != nil {
		panic("problem running manager")
	}
	setupLog.Info("Cleaning up cloned directories")

	for _, path := range gitpaths.GetValues() {
		err := os.RemoveAll(path)
		if err != nil {
			setupLog.Error(err, "failed to cleanup directory")
		}
		setupLog.Info("cleaning directory", "directory", path)
	}
	return nil
}

func newDashboardCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "dashboard",
		Short: "GitOps Promoter dashboard",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Println("Dashboard is not implemented yet.")
		},
	}
}

func newCommand() *cobra.Command {
	var clientConfig clientcmd.ClientConfig

	opts := zap.Options{
		Development: true,
		TimeEncoder: zapcore.RFC3339NanoTimeEncoder,
	}

	cmd := &cobra.Command{
		Use:   "promoter",
		Short: "GitOps Promoter",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
		},
	}

	// Zap only operates on go-type flags. Cobra doesn't give us direct access to those flags.
	// So we apply the zap flags to a temp go flags set and then transfer them to the cobra flags.
	tmpZapFlagSet := flag.NewFlagSet("", flag.ContinueOnError)
	opts.BindFlags(tmpZapFlagSet)
	// Transfer flags from the temporary FlagSet to cobra's pflag.FlagSet
	tmpZapFlagSet.VisitAll(func(f *flag.Flag) {
		cmd.PersistentFlags().AddGoFlag(f)
	})

	clientConfig = addKubectlFlags(cmd.PersistentFlags())
	cmd.AddCommand(newControllerCommand(clientConfig))
	cmd.AddCommand(newDashboardCommand())
	return cmd
}

func main() {
	if err := newCommand().Execute(); err != nil {
		os.Exit(1)
	}
}

func addKubectlFlags(flags *pflag.FlagSet) clientcmd.ClientConfig {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.DefaultClientConfig = &clientcmd.DefaultClientConfig
	overrides := clientcmd.ConfigOverrides{}
	kflags := clientcmd.RecommendedConfigOverrideFlags("")
	flags.StringVar(&loadingRules.ExplicitPath, "kubeconfig", "", "Path to a kube config. Only required if out-of-cluster")
	clientcmd.BindOverrideFlags(&overrides, flags, kflags)
	return clientcmd.NewInteractiveDeferredLoadingClientConfig(loadingRules, &overrides, os.Stdin)
}
