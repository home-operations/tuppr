package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	goruntime "runtime"
	"strings"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/open-policy-agent/cert-controller/pkg/rotator"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	tupperv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/alerting"
	"github.com/home-operations/tuppr/internal/controller/kubernetesupgrade"
	"github.com/home-operations/tuppr/internal/controller/talosupgrade"
	"github.com/home-operations/tuppr/internal/metrics"
	"github.com/home-operations/tuppr/internal/notification"
	kuberneteswebhook "github.com/home-operations/tuppr/internal/webhook/kubernetesupgrade"
	taloswebhook "github.com/home-operations/tuppr/internal/webhook/talosupgrade"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
	certDir  = "/tmp/k8s-webhook-server/serving-certs"
)

// Populated via -ldflags at build time.
var (
	version = "dev"
	commit  = "unknown"
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
	var enableHTTP2 bool
	var talosConfigSecret string
	var logLevel string
	var tlsOpts []func(*tls.Config)

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8081", "The address the metrics endpoint binds to "+
		"(plain HTTP; the org port standard). The /healthz and /readyz probes are co-hosted on this "+
		"same listener, so the controller exposes a single operational port.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
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
	flag.StringVar(&logLevel, "log-level", "info",
		"Log level for the controller (debug, info)")

	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	switch strings.ToLower(logLevel) {
	case "debug":
		opts.Development = true
	default:
		opts.Development = false
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Get controller namespace from environment
	controllerNamespace := os.Getenv("CONTROLLER_NAMESPACE")
	if controllerNamespace == "" {
		controllerNamespace = "tuppr-system" // Default namespace
	}

	// Get controller node name and pod name from environment (injected via downward API)
	controllerNodeName := os.Getenv("CONTROLLER_NODE_NAME")
	controllerPodName := os.Getenv("CONTROLLER_POD_NAME")

	// Default the upgrade Job's --endpoint to the apiserver ClusterIP so it
	// keeps working when CoreDNS is drained mid-upgrade.
	kubernetesAPIEndpoint := ""
	host := os.Getenv("KUBERNETES_SERVICE_HOST")
	port := os.Getenv("KUBERNETES_SERVICE_PORT_HTTPS")
	if host != "" && port != "" {
		kubernetesAPIEndpoint = fmt.Sprintf("https://%s", net.JoinHostPort(host, port))
	}

	reporter := metrics.NewReporter()
	reporter.RecordBuildInfo(version, commit, goruntime.Version())
	reporter.InitializeAtBoot()

	notificationURL := os.Getenv("NOTIFICATION_URL")
	notifier := notification.NewAppriseNotifier(notificationURL)
	notificationsEnabled := notifier != nil

	notificationRenderer, err := notification.NewRenderer(
		os.Getenv("NOTIFICATION_TITLE_TEMPLATE"),
		os.Getenv("NOTIFICATION_MESSAGE_TEMPLATE"),
	)
	if err != nil {
		setupLog.Error(err, "invalid notification template")
		os.Exit(1)
	}

	var silencer alerting.Silencer
	if alertmanagerAddress := os.Getenv("ALERTMANAGER_ADDRESS"); alertmanagerAddress != "" {
		headers, err := alerting.LoadHeadersDir(os.Getenv("ALERTMANAGER_HEADERS_DIR"))
		if err != nil {
			setupLog.Error(err, "invalid alertmanager headers")
			os.Exit(1)
		}
		silencer = alerting.NewClient(alertmanagerAddress, headers)
	}

	if metricsServiceName == "" {
		metricsServiceName = "tuppr-metrics-service"
	}

	setupLog.Info("Starting tuppr controller manager",
		"talosconfig-secret", talosConfigSecret,
		"controller-namespace", controllerNamespace)
	if notificationsEnabled {
		setupLog.Info("Notification configuration loaded",
			"notifications_enabled", true,
		)
	}

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

	// The org port standard for controllers: one plain-HTTP operational listener
	// (:8081) serving /metrics plus the /healthz and /readyz probes (registered as
	// ExtraHandlers after the manager is built). Plain HTTP is required for the
	// co-host: the metrics server wraps every ExtraHandler in its TLS/authn
	// FilterProvider, which a kubelet's unauthenticated probe can't pass — so
	// there is no secure-metrics option; restrict the port with a NetworkPolicy.
	metricsServerOptions := metricsserver.Options{
		BindAddress: metricsAddr,
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:        scheme,
		Metrics:       metricsServerOptions,
		WebhookServer: webhookServer,
		// The dedicated health-probe server is disabled; probes are co-hosted on
		// the metrics listener above.
		HealthProbeBindAddress: "0",
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

	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&corev1.Pod{},
		"spec.nodeName",
		func(obj client.Object) []string {
			pod, ok := obj.(*corev1.Pod)
			if !ok || pod.Spec.NodeName == "" {
				return nil
			}
			return []string{pod.Spec.NodeName}
		},
	); err != nil {
		setupLog.Error(err, "unable to create index", "field", "Pod.spec.nodeName")
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

	talosRecorder := mgr.GetEventRecorderFor("talosupgrade-controller")           //nolint:staticcheck
	kubernetesRecorder := mgr.GetEventRecorderFor("kubernetesupgrade-controller") //nolint:staticcheck

	if err := (&talosupgrade.Reconciler{
		Client:              mgr.GetClient(),
		Scheme:              mgr.GetScheme(),
		TalosConfigSecret:   talosConfigSecret,
		ControllerNamespace: controllerNamespace,
		ControllerNodeName:  controllerNodeName,
		ControllerPodName:   controllerPodName,
		Notifier:            notifier,
		Renderer:            notificationRenderer,
		Silencer:            silencer,
		Recorder:            talosRecorder,
		MetricsReporter:     reporter,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TalosUpgrade")
		os.Exit(1)
	}
	if err := (&kubernetesupgrade.Reconciler{
		Client:              mgr.GetClient(),
		Scheme:              mgr.GetScheme(),
		TalosConfigSecret:   talosConfigSecret,
		ControllerNamespace: controllerNamespace,
		ControllerNodeName:  controllerNodeName,
		DefaultEndpoint:     kubernetesAPIEndpoint,
		Recorder:            kubernetesRecorder,
		MetricsReporter:     reporter,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KubernetesUpgrade")
		os.Exit(1)
	}

	if err := mgr.Add(&metrics.InventoryRefresher{
		Client:   mgr.GetClient(),
		Cache:    mgr.GetCache(),
		Reporter: reporter,
		Interval: 30 * time.Second,
	}); err != nil {
		setupLog.Error(err, "unable to register inventory metrics refresher")
		os.Exit(1)
	}

	go func() {
		setupLog.Info("waiting for cert rotation setup to register webhooks")
		<-certSetupFinished // This blocks until the rotator creates the files
		setupLog.Info("cert rotation setup complete, registering webhooks")

		// +kubebuilder:scaffold:builder
		if err := (&taloswebhook.Validator{
			Client:                 mgr.GetClient(),
			TalosConfigSecret:      talosConfigSecret,
			Namespace:              controllerNamespace,
			AlertmanagerConfigured: silencer != nil,
		}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "TalosUpgrade")
			os.Exit(1)
		}

		if err := (&kuberneteswebhook.Validator{
			Client:            mgr.GetClient(),
			TalosConfigSecret: talosConfigSecret,
			Namespace:         controllerNamespace,
		}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "KubernetesUpgrade")
			os.Exit(1)
		}
		setupLog.Info("webhooks registered successfully")
	}()

	// Health and readiness checks. healthz is a liveness ping; readyz reports ready once
	// the webhook server has started so the cert rotator has wired up the webhooks.
	var readyzCheck healthz.Checker = func(req *http.Request) error {
		return mgr.GetWebhookServer().StartedChecker()(req)
	}

	// Serve the probes on the (plain HTTP) metrics listener so the controller exposes
	// a single port. mgr.AddHealthzCheck/AddReadyzCheck only feed controller-runtime's
	// dedicated health-probe server, which is disabled, so register the checks as
	// ExtraHandlers on the metrics server instead. healthz.CheckHandler returns 200 when
	// the checker passes and 500 otherwise — the contract a kubelet HTTP probe expects.
	if err := mgr.AddMetricsServerExtraHandler("/healthz", healthz.CheckHandler{Checker: healthz.Ping}); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddMetricsServerExtraHandler("/readyz", healthz.CheckHandler{Checker: readyzCheck}); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}
	setupLog.Info("serving health and readiness probes on the metrics listener", "bind-address", metricsAddr)

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
