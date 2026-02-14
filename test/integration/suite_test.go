package integration

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/constants"
	"github.com/home-operations/tuppr/internal/controller"
)

var (
	cfg              *rest.Config
	k8sClient        client.Client
	testEnv          *envtest.Environment
	ctx              context.Context
	cancel           context.CancelFunc
	k8sManager       ctrl.Manager
	jobSimulatorStop context.CancelFunc

	// Shared mocks for tests to access
	sharedMockTalos   *mockTalosClient
	sharedMockHealth  *mockHealthChecker
	sharedMockVersion *mockVersionGetter
)

// Helper functions to access mocks from tests
func getMockTalosClient() *mockTalosClient {
	return sharedMockTalos
}

func getMockHealthChecker() *mockHealthChecker {
	return sharedMockHealth
}

func getMockVersionGetter() *mockVersionGetter {
	return sharedMockVersion
}

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Test Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.Background())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = tupprv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	By("creating manager")
	k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme:  scheme.Scheme,
		Metrics: ctrl.Options{}.Metrics,
	})
	Expect(err).ToNot(HaveOccurred())

	By("creating test namespace and talosconfig secret")
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tuppr-system",
		},
	}
	Expect(k8sClient.Create(ctx, ns)).To(Succeed())

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tuppr",
			Namespace: "tuppr-system",
		},
		Data: map[string][]byte{
			constants.TalosSecretKey: []byte(validTalosConfig()),
		},
	}
	Expect(k8sClient.Create(ctx, secret)).To(Succeed())

	By("creating mock dependencies")
	sharedMockTalos = &mockTalosClient{
		nodeVersions:  make(map[string]string),
		installImages: make(map[string]string),
	}
	sharedMockHealth = &mockHealthChecker{}
	sharedMockVersion = &mockVersionGetter{version: "v1.33.0"}

	mockTalos := sharedMockTalos
	mockHealth := sharedMockHealth
	mockVersion := sharedMockVersion

	By("setting up TalosUpgrade controller with mocks")
	talosReconciler := &controller.TalosUpgradeReconciler{
		Client:              k8sManager.GetClient(),
		Scheme:              k8sManager.GetScheme(),
		TalosConfigSecret:   "tuppr",
		ControllerNamespace: "tuppr-system",
		TalosClient:         mockTalos,
		HealthChecker:       mockHealth,
	}
	err = talosReconciler.SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	By("setting up KubernetesUpgrade controller with mocks")
	k8sReconciler := &controller.KubernetesUpgradeReconciler{
		Client:              k8sManager.GetClient(),
		Scheme:              k8sManager.GetScheme(),
		TalosConfigSecret:   "tuppr",
		ControllerNamespace: "tuppr-system",
		TalosClient:         mockTalos,
		HealthChecker:       mockHealth,
		VersionGetter:       mockVersion,
	}
	err = k8sReconciler.SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	By("starting job simulator")
	var jobSimulatorCtx context.Context
	jobSimulatorCtx, jobSimulatorStop = context.WithCancel(ctx)
	go runJobSimulator(jobSimulatorCtx, k8sClient)

	By("starting manager")
	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()
})

var _ = AfterSuite(func() {
	By("stopping job simulator")
	if jobSimulatorStop != nil {
		jobSimulatorStop()
	}

	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

func validTalosConfig() string {
	return `context: default
contexts:
  default:
    endpoints:
      - https://10.0.0.1:50000
    ca: ""
    crt: ""
    key: ""
`
}

// runJobSimulator watches for Jobs and simulates their completion
func runJobSimulator(ctx context.Context, k8sClient client.Client) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			jobList := &batchv1.JobList{}
			if err := k8sClient.List(ctx, jobList); err != nil {
				continue
			}

			for i := range jobList.Items {
				job := &jobList.Items[i]

				// Skip jobs that are already complete or failed
				if isJobComplete(job) || isJobFailed(job) {
					continue
				}

				// Simulate job completion after a short delay
				if job.Status.StartTime != nil && time.Since(job.Status.StartTime.Time) > 1*time.Second {
					job.Status.Succeeded = 1
					job.Status.Conditions = []batchv1.JobCondition{
						{
							Type:               batchv1.JobComplete,
							Status:             corev1.ConditionTrue,
							LastTransitionTime: metav1.Now(),
						},
					}
					_ = k8sClient.Status().Update(ctx, job)
				} else if job.Status.StartTime == nil {
					// Set start time if not set
					now := metav1.Now()
					job.Status.StartTime = &now
					_ = k8sClient.Status().Update(ctx, job)
				}
			}
		}
	}
}

func isJobComplete(job *batchv1.Job) bool {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func isJobFailed(job *batchv1.Job) bool {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
