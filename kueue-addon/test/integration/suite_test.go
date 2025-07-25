package integration

import (
	"context"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2" // Ginkgo BDD testing framework
	"github.com/onsi/gomega"    // Gomega matcher/assertion library
	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"github.com/openshift/library-go/pkg/operator/events"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/clock"
	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/hub"
	clusterv1client "open-cluster-management.io/api/client/cluster/clientset/versioned"
	permissionclientset "open-cluster-management.io/cluster-permission/client/clientset/versioned"
	msaclientset "open-cluster-management.io/managed-serviceaccount/pkg/generated/clientset/versioned"
	"sigs.k8s.io/controller-runtime/pkg/envtest" // Kubernetes envtest
	kueueclientset "sigs.k8s.io/kueue/client-go/clientset/versioned"
)

const (
	eventuallyTimeout  = 30 // seconds
	eventuallyInterval = 1  // seconds
)

var (
	hubClusterClient    clusterv1client.Interface
	hubKueueClient      kueueclientset.Interface
	hubKubeClient       kubernetes.Interface
	hubPermissionClient permissionclientset.Interface
	hubMSAClient        msaclientset.Interface
)

var testEnv *envtest.Environment
var cancel context.CancelFunc
var mgrContext context.Context
var CRDPaths = []string{
	"./vendor/open-cluster-management.io/api/cluster/v1/0000_00_clusters.open-cluster-management.io_managedclusters.crd.yaml",
	"./vendor/open-cluster-management.io/api/cluster/v1beta1/0000_02_clusters.open-cluster-management.io_placements.crd.yaml",
	"./vendor/open-cluster-management.io/api/cluster/v1beta1/0000_03_clusters.open-cluster-management.io_placementdecisions.crd.yaml",
	"./test/integration/testdeps/kueue/crd.yaml",
	"./test/integration/testdeps/managed-serviceaccount/crd.yaml",
	"./test/integration/testdeps/cluster-permission/crd.yaml",
}

func TestIntegration(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Kueue Addon Integration Suite")
}

var _ = ginkgo.BeforeSuite(func() {
	ginkgo.By("bootstrapping test environment")

	// start a kube-apiserver
	testEnv = &envtest.Environment{
		ErrorIfCRDPathMissing: true,
		CRDDirectoryPaths:     CRDPaths,
	}

	cfg, err := testEnv.Start()
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	gomega.Expect(cfg).ToNot(gomega.BeNil())

	mgrContext, cancel = context.WithCancel(context.TODO())

	// Initialize clients
	hubClusterClient, err = clusterv1client.NewForConfig(cfg)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	hubKueueClient, err = kueueclientset.NewForConfig(cfg)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	hubKubeClient, err = kubernetes.NewForConfig(cfg)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	hubPermissionClient, err = permissionclientset.NewForConfig(cfg)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	hubMSAClient, err = msaclientset.NewForConfig(cfg)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	// Start the kueue-addon controllers
	ginkgo.By("starting kueue-addon controllers")
	controllerContext := &controllercmd.ControllerContext{
		KubeConfig:    cfg,
		EventRecorder: events.NewInMemoryRecorder("kueue-addon", clock.RealClock{}), // We'll use a simple recorder for tests
	}

	go func() {
		err := hub.RunControllerManager(mgrContext, controllerContext)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}()

	// Wait a bit for controllers to start
	time.Sleep(2 * time.Second)
})

var _ = ginkgo.AfterSuite(func() {
	ginkgo.By("tearing down the test environment")
	if cancel != nil {
		cancel()
	}
	if testEnv != nil {
		err := testEnv.Stop()
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}
})
