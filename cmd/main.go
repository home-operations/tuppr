package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/open-policy-agent/cert-controller/pkg/rotator"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	tupperv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/controller"
	tupprwebhook "github.com/home-operations/tuppr/internal/webhook"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
	certDir  = "/tmp/k8s-webhook-server/serving-certs"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(tupperv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// nolint:gocyclo
func main() {
	var metricsAddr string
	var metricsServiceName string
	var webhookConfigName, webhookServiceName, webhookSecretName string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var talosConfigSecret string
	var tlsOpts []func(*tls.Config)

	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.StringVar(&talosConfigSecret, "talosconfig-secret", "tuppr",
		"The name of the secret containing talos configuration")
	flag.StringVar(&metricsServiceName, "metrics-service-name", "",
		"The name of the service name of the metric server")
	flag.StringVar(&webhookConfigName, "webhook-config-name", "",
		"The name of the ValidatingWebhookConfiguration to patch with CA bundle")
	flag.StringVar(&webhookServiceName, "webhook-service-name", "",
		"The DNS name of the webhook service (e.g. tuppr-webhook-service)")
	flag.StringVar(&webhookSecretName, "webhook-secret-name", "",
		"The name of the Secret to store webhook certificates")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Get controller namespace from environment
	controllerNamespace := os.Getenv("CONTROLLER_NAMESPACE")
	if controllerNamespace == "" {
		controllerNamespace = "tuppr-system" // Default namespace
	}

	if metricsServiceName == "" {
		metricsServiceName = "tuppr-metrics-service"
	}

	setupLog.Info("Starting tuppr controller manager",
		"talosconfig-secret", talosConfigSecret,
		"controller-namespace", controllerNamespace)

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

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "bea89bcd.home-operations.com",
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
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Set up self-signed certificate rotation for webhooks
	certSetupFinished := make(chan struct{})
	dnsName := fmt.Sprintf("%s.%s.svc", webhookServiceName, controllerNamespace)
	setupLog.Info("setting up cert rotation",
		"webhook-config", webhookConfigName,
		"dns-name", dnsName,
		"secret", webhookSecretName,
	)
	if err := rotator.AddRotator(mgr, &rotator.CertRotator{
		SecretKey: types.NamespacedName{
			Namespace: controllerNamespace,
			Name:      webhookSecretName,
		},
		CertDir:        certDir,
		CAName:         "tuppr-ca",
		CAOrganization: "tuppr",
		DNSName:        dnsName,
		ExtraDNSNames: []string{
			fmt.Sprintf("%s.%s.svc.cluster.local", webhookServiceName, controllerNamespace),
			fmt.Sprintf("%s.%s.svc", metricsServiceName, controllerNamespace),
		},
		IsReady:              certSetupFinished,
		EnableReadinessCheck: true,
		Webhooks: []rotator.WebhookInfo{
			{
				Name: webhookConfigName,
				Type: rotator.Validating,
			},
		},
	}); err != nil {
		setupLog.Error(err, "unable to set up cert rotation")
		os.Exit(1)
	}

	setupLog.Info("Setting up controllers")

	if err := (&controller.TalosUpgradeReconciler{
		Client:              mgr.GetClient(),
		Scheme:              mgr.GetScheme(),
		TalosConfigSecret:   talosConfigSecret,
		ControllerNamespace: controllerNamespace,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TalosUpgrade")
		os.Exit(1)
	}
	if err := (&controller.KubernetesUpgradeReconciler{
		Client:              mgr.GetClient(),
		Scheme:              mgr.GetScheme(),
		TalosConfigSecret:   talosConfigSecret,
		ControllerNamespace: controllerNamespace,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KubernetesUpgrade")
		os.Exit(1)
	}

	go func() {
		setupLog.Info("waiting for cert rotation setup to register webhooks")
		<-certSetupFinished // This blocks until the rotator creates the files
		setupLog.Info("cert rotation setup complete, registering webhooks")

		// +kubebuilder:scaffold:builder
		if err := (&tupprwebhook.TalosUpgradeValidator{
			Client:            mgr.GetClient(),
			TalosConfigSecret: talosConfigSecret, // Pass the secret name!
		}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "TalosUpgrade")
			os.Exit(1)
		}

		if err := (&tupprwebhook.KubernetesUpgradeValidator{
			Client:            mgr.GetClient(),
			TalosConfigSecret: talosConfigSecret, // Pass the secret name!
		}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "KubernetesUpgrade")
			os.Exit(1)
		}
		setupLog.Info("webhooks registered successfully")
	}()

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}

	if err := mgr.AddReadyzCheck("readyz", func(req *http.Request) error {
		return mgr.GetWebhookServer().StartedChecker()(req)
	}); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
