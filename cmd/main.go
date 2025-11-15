package main

import (
	"crypto/tls"
	"flag"
	"os"

	"github.com/go-logr/zerologr"
	"github.com/rs/zerolog"

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	otcv1alpha1 "github.com/peertech.de/otc-operator/api/v1alpha1"
	"github.com/peertech.de/otc-operator/internal/controller"
	"github.com/peertech.de/otc-operator/internal/version"
	webhookv1alpha1 "github.com/peertech.de/otc-operator/internal/webhook/v1alpha1"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(otcv1alpha1.AddToScheme(scheme))
}

// nolint:gocyclo
func main() {
	var logLevel string
	var logFormat string
	var metricsAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var tlsOpts []func(*tls.Config)
	flag.StringVar(
		&metricsAddr,
		"metrics-bind-address",
		"0",
		"The address the metrics endpoint binds to. "+
			"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.",
	)
	flag.StringVar(
		&probeAddr,
		"health-probe-bind-address",
		":8081",
		"The address the probe endpoint binds to.",
	)
	flag.BoolVar(
		&enableLeaderElection,
		"leader-elect",
		false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(
		&secureMetrics,
		"metrics-secure",
		true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.",
	)
	flag.StringVar(
		&webhookCertPath,
		"webhook-cert-path",
		"",
		"The directory that contains the webhook certificate.",
	)
	flag.StringVar(
		&webhookCertName,
		"webhook-cert-name",
		"tls.crt",
		"The name of the webhook certificate file.",
	)
	flag.StringVar(
		&webhookCertKey,
		"webhook-cert-key",
		"tls.key",
		"The name of the webhook key file.",
	)
	flag.StringVar(
		&metricsCertPath,
		"metrics-cert-path",
		"",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(
		&metricsCertName,
		"metrics-cert-name",
		"tls.crt",
		"The name of the metrics server certificate file.",
	)
	flag.StringVar(
		&metricsCertKey,
		"metrics-cert-key",
		"tls.key",
		"The name of the metrics server key file.",
	)
	flag.BoolVar(
		&enableHTTP2,
		"enable-http2",
		false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")

	flag.StringVar(
		&logLevel,
		"log-level",
		"info",
		"Log level [trace|debug|info|warn|error|fatal|panic]",
	)
	flag.StringVar(
		&logFormat,
		"log-format",
		"json",
		"Log format [json|console]",
	)
	flag.Parse()

	// Create logger.
	logger := configureLogger(logLevel, logFormat)

	// Set controller-runtime to use the zerolog adapter.
	ctrl.SetLogger(zerologr.New(&logger))

	// Create setup logger.
	setupLog := logger.With().Str("component", "setup").Logger()

	setupLog.Info().
		Str("version", version.Version).
		Str("commit", version.Commit).
		Msg("Starting Operator...")

	// if the enable-http2 flag is false (the default), http/2 should be
	// disabled due to its vulnerabilities. More specifically, disabling http/2
	// will prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	//  - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	//  - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info().Msg("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Initial webhook TLS options
	webhookTLSOpts := tlsOpts
	webhookServerOptions := webhook.Options{
		TLSOpts: webhookTLSOpts,
	}

	if len(webhookCertPath) > 0 {
		setupLog.Info().
			Str("webhook-cert-path", webhookCertPath).
			Str("webhook-cert-name", webhookCertName).
			Str("webhook-cert-key", webhookCertKey).
			Msg("Initializing webhook certificate watcher using provided certificates")

		webhookServerOptions.CertDir = webhookCertPath
		webhookServerOptions.CertName = webhookCertName
		webhookServerOptions.KeyName = webhookCertKey
	}

	webhookServer := webhook.NewServer(webhookServerOptions)

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The
	// Metrics options configure the server.
	//
	// More info:
	//  - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/metrics/server
	//  - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with
		// authn/authz. These configurations ensure that only authorized users
		// and service accounts can access the metrics endpoint. The RBAC are
		// configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	// If the certificate is not specified, controller-runtime will
	// automatically generate self-signed certificates for the metrics server.
	// While convenient for development and testing, this setup is not
	// recommended for production.
	//
	// TODO(user): If you enable certManager, uncomment the following lines:
	//   - [METRICS-WITH-CERTS] at config/default/kustomization.yaml to generate and use certificates
	// 	managed by cert-manager for the metrics server.
	//   - [PROMETHEUS-WITH-CERTS] at config/prometheus/kustomization.yaml for TLS certification.
	if len(metricsCertPath) > 0 {
		setupLog.Info().
			Str("metrics-cert-path", metricsCertPath).
			Str("metrics-cert-name", metricsCertName).
			Str("metrics-cert-key", metricsCertKey).
			Msg("Initializing metrics certificate watcher using provided certificates")

		metricsServerOptions.CertDir = metricsCertPath
		metricsServerOptions.CertName = metricsCertName
		metricsServerOptions.KeyName = metricsCertKey
	}

	mgr, err := ctrl.NewManager(
		ctrl.GetConfigOrDie(),
		ctrl.Options{
			Scheme:                 scheme,
			Metrics:                metricsServerOptions,
			WebhookServer:          webhookServer,
			HealthProbeBindAddress: probeAddr,
			LeaderElection:         enableLeaderElection,
			LeaderElectionID:       "otc-operator-lock",
		},
	)
	if err != nil {
		setupLog.Fatal().Err(err).Msg("Failed to start manager")
	}

	// Create the a provider cache, which gets shared among all controllers.
	providers := controller.NewProviderCache(mgr.GetClient(), logger)

	// Create Provider controller.
	providerConfigReconcicler := controller.NewProviderConfigReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		logger,
		providers,
	)
	if err := providerConfigReconcicler.SetupWithManager(mgr); err != nil {
		setupLog.Fatal().Err(err).Msg("Failed to create provider config controller")
	}

	// Register Provider Config webhook
	if err := webhookv1alpha1.SetupProviderConfigWebhookWithManager(mgr); err != nil {
		setupLog.Fatal().Err(err).Msg("Failed to create provider config webhook")
	}

	// Create Network controller.
	networkReconciler := controller.NewNetworkReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		logger,
		providers,
	)
	if err := networkReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Fatal().Err(err).Msg("Failed to create Network controller")
	}

	// Register Network webhook
	if err := webhookv1alpha1.SetupNetworkWebhookWithManager(mgr); err != nil {
		setupLog.Fatal().Err(err).Msg("Failed to create Network webhook")
	}

	// Create Subnet controller.
	subnetReconciler := controller.NewSubnetReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		logger,
		providers,
	)
	if err := subnetReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Fatal().Err(err).Msg("Failed to create Subnet controller")
	}

	// Register Subnet webhook
	if err := webhookv1alpha1.SetupSubnetWebhookWithManager(mgr); err != nil {
		setupLog.Fatal().Err(err).Msg("Failed to create Subnet webhook")
	}

	// Create Public IP controller.
	publicIPReconciler := controller.NewPublicIPReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		logger,
		providers,
	)
	if err := publicIPReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Fatal().Err(err).Msg("Failed to create Public IP controller")
	}

	// Register Public IP webhook
	if err := webhookv1alpha1.SetupPublicIPWebhookWithManager(mgr); err != nil {
		setupLog.Fatal().Err(err).Msg("Failed to create Public IP webhook")
	}

	// Create NAT gateway controller.
	natGatewayReconciler := controller.NewNATGatewayReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		logger,
		providers,
	)
	if err := natGatewayReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Fatal().Err(err).Msg("Failed to create NAT gateway controller")
	}

	// Register NAT gateway webhook
	if err := webhookv1alpha1.SetupNATGatewayWebhookWithManager(mgr); err != nil {
		setupLog.Fatal().Err(err).Msg("Failed to create NAT gateway webhook")
	}

	// Create SNAT rule controller.
	snatRuleReconciler := controller.NewSNATRuleReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		logger,
		providers,
	)
	if err := snatRuleReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Fatal().Err(err).Msg("Failed to create SNAT rule controller")
	}

	// Register SNAT rule webhook
	if err := webhookv1alpha1.SetupSNATRuleWebhookWithManager(mgr); err != nil {
		setupLog.Fatal().Err(err).Msg("Failed to create SNAT rule webhook")
	}

	// Create Security Group controller.
	securityGroupReconciler := controller.NewSecurityGroupReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		logger,
		providers,
	)
	if err := securityGroupReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Fatal().Err(err).Msg("Failed to create Security Group controller")
	}

	// Register Security Group webhook
	if err := webhookv1alpha1.SetupSecurityGroupWebhookWithManager(mgr); err != nil {
		setupLog.Fatal().Err(err).Msg("Failed to create Security Group webhook")
	}

	// Create Security Group rule controller.
	securityGroupRuleReconciler := controller.NewSecurityGroupRuleReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		logger,
		providers,
	)
	if err := securityGroupRuleReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Fatal().Err(err).Msg("Failed to create Security Group rule controller")
	}

	// Register Security Group rule webhook
	if err := webhookv1alpha1.SetupSecurityGroupRuleWebhookWithManager(mgr); err != nil {
		setupLog.Fatal().Err(err).Msg("Failed to create Security Group rule webhook")
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Fatal().Err(err).Msg("Failed to set up health check")
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Fatal().Err(err).Msg("Failed to set up ready check")
	}

	setupLog.Info().Msg("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Fatal().Err(err).Msg("Failed to start manager")
	}
}

func configureLogger(level, format string) zerolog.Logger {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)

	var logger zerolog.Logger
	switch format {
	case "console":
		logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
			With().
			Timestamp().
			Caller().
			Logger()
	default:
		logger = zerolog.New(os.Stdout).
			With().
			Timestamp().
			Caller().
			Logger()
	}

	return logger
}
