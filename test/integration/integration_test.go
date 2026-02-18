package integration

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	infrastructurev1beta1 "github.com/fabiendupont/cluster-api-provider-nvidia-bmm/api/v1beta1"
	"github.com/fabiendupont/cluster-api-provider-nvidia-bmm/internal/controller"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
			filepath.Join("..", "..", "config", "crd", "external"),
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = infrastructurev1beta1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = clusterv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Start controllers
	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // Disable metrics server in tests
		},
		HealthProbeBindAddress: "0", // Disable health probe in tests
	})
	Expect(err).ToNot(HaveOccurred())

	err = (&controller.NvidiaBMMClusterReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(ctx, k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&controller.NvidiaBMMMachineReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("NvidiaBMMCluster Integration", func() {
	var (
		namespace        *corev1.Namespace
		cluster          *clusterv1.Cluster
		nvidiaBmmCluster *infrastructurev1beta1.NvidiaBMMCluster
		credSecret       *corev1.Secret
	)

	BeforeEach(func() {
		// Create test namespace
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
		}
		Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

		// Create credentials secret
		credSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nvidia-bmm-creds",
				Namespace: namespace.Name,
			},
			Data: map[string][]byte{
				"endpoint": []byte("https://api.test.com"),
				"orgName":  []byte("test-org"),
				"token":    []byte("test-token"),
			},
		}
		Expect(k8sClient.Create(ctx, credSecret)).To(Succeed())

		// Create CAPI Cluster
		cluster = &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: namespace.Name,
			},
			Spec: clusterv1.ClusterSpec{
				InfrastructureRef: clusterv1.ContractVersionedObjectReference{
					APIGroup: "infrastructure.cluster.x-k8s.io",
					Kind:     "NvidiaBMMCluster",
					Name:     "test-cluster",
				},
			},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())

		// Create NvidiaBMMCluster
		nvidiaBmmCluster = &infrastructurev1beta1.NvidiaBMMCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: namespace.Name,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.String(),
						Kind:       "Cluster",
						Name:       cluster.Name,
						UID:        cluster.UID,
					},
				},
			},
			Spec: infrastructurev1beta1.NvidiaBMMClusterSpec{
				SiteRef: infrastructurev1beta1.SiteReference{
					ID: "8a880c71-fe4b-4e43-9e24-ebfcb8a84c5f",
				},
				TenantID: "b013708a-99f0-47b2-a630-cabb4ae1d3df",
				VPC: infrastructurev1beta1.VPCSpec{
					Name:                      "test-vpc",
					NetworkVirtualizationType: "ETHERNET_VIRTUALIZER",
				},
				Subnets: []infrastructurev1beta1.SubnetSpec{
					{
						Name: "control-plane",
						CIDR: "10.100.1.0/24",
						Role: "control-plane",
					},
					{
						Name: "worker",
						CIDR: "10.100.2.0/24",
						Role: "worker",
					},
				},
				Authentication: infrastructurev1beta1.AuthenticationSpec{
					SecretRef: corev1.SecretReference{
						Name:      credSecret.Name,
						Namespace: namespace.Name,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, nvidiaBmmCluster)).To(Succeed())
	})

	AfterEach(func() {
		// Clean up namespace
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
	})

	It("should add finalizer to NvidiaBMMCluster", func() {
		Eventually(func() []string {
			updated := &infrastructurev1beta1.NvidiaBMMCluster{}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(nvidiaBmmCluster), updated)
			if err != nil {
				return nil
			}
			return updated.Finalizers
		}, 10*time.Second, 500*time.Millisecond).Should(ContainElement(controller.NvidiaBMMClusterFinalizer))
	})

	It("should handle missing owner cluster gracefully", func() {
		orphanCluster := &infrastructurev1beta1.NvidiaBMMCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "orphan-cluster",
				Namespace: namespace.Name,
			},
			Spec: infrastructurev1beta1.NvidiaBMMClusterSpec{
				SiteRef: infrastructurev1beta1.SiteReference{
					ID: "8a880c71-fe4b-4e43-9e24-ebfcb8a84c5f",
				},
				TenantID: "b013708a-99f0-47b2-a630-cabb4ae1d3df",
				VPC: infrastructurev1beta1.VPCSpec{
					Name:                      "orphan-vpc",
					NetworkVirtualizationType: "ETHERNET_VIRTUALIZER",
				},
				Subnets: []infrastructurev1beta1.SubnetSpec{
					{
						Name: "default",
						CIDR: "10.200.1.0/24",
					},
				},
				Authentication: infrastructurev1beta1.AuthenticationSpec{
					SecretRef: corev1.SecretReference{
						Name:      credSecret.Name,
						Namespace: namespace.Name,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, orphanCluster)).To(Succeed())

		// Should not panic or error out
		Consistently(func() bool {
			updated := &infrastructurev1beta1.NvidiaBMMCluster{}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(orphanCluster), updated)
			return err == nil
		}, 3*time.Second, 500*time.Millisecond).Should(BeTrue())
	})
})

var _ = Describe("NvidiaBMMMachine Integration", func() {
	var (
		namespace        *corev1.Namespace
		cluster          *clusterv1.Cluster
		nvidiaBmmCluster *infrastructurev1beta1.NvidiaBMMCluster
		machine          *clusterv1.Machine
		nvidiaBmmMachine *infrastructurev1beta1.NvidiaBMMMachine
		credSecret       *corev1.Secret
		bootstrapSecret  *corev1.Secret
	)

	BeforeEach(func() {
		// Create test namespace
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
		}
		Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

		// Create credentials secret
		credSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nvidia-bmm-creds",
				Namespace: namespace.Name,
			},
			Data: map[string][]byte{
				"endpoint": []byte("https://api.test.com"),
				"orgName":  []byte("test-org"),
				"token":    []byte("test-token"),
			},
		}
		Expect(k8sClient.Create(ctx, credSecret)).To(Succeed())

		// Create bootstrap secret
		bootstrapSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bootstrap-data",
				Namespace: namespace.Name,
			},
			Data: map[string][]byte{
				"value": []byte("#!/bin/bash\nkubeadm join..."),
			},
		}
		Expect(k8sClient.Create(ctx, bootstrapSecret)).To(Succeed())

		// Create CAPI Cluster
		cluster = &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: namespace.Name,
			},
			Spec: clusterv1.ClusterSpec{
				InfrastructureRef: clusterv1.ContractVersionedObjectReference{
					APIGroup: "infrastructure.cluster.x-k8s.io",
					Kind:     "NvidiaBMMCluster",
					Name:     "test-cluster",
				},
			},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())

		// Create NvidiaBMMCluster (already ready)
		nvidiaBmmCluster = &infrastructurev1beta1.NvidiaBMMCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: namespace.Name,
			},
			Spec: infrastructurev1beta1.NvidiaBMMClusterSpec{
				SiteRef: infrastructurev1beta1.SiteReference{
					ID: "8a880c71-fe4b-4e43-9e24-ebfcb8a84c5f",
				},
				TenantID: "b013708a-99f0-47b2-a630-cabb4ae1d3df",
				VPC: infrastructurev1beta1.VPCSpec{
					Name:                      "test-vpc",
					NetworkVirtualizationType: "ETHERNET_VIRTUALIZER",
				},
				Subnets: []infrastructurev1beta1.SubnetSpec{
					{
						Name: "control-plane",
						CIDR: "10.100.1.0/24",
						Role: "control-plane",
					},
				},
				Authentication: infrastructurev1beta1.AuthenticationSpec{
					SecretRef: corev1.SecretReference{
						Name:      credSecret.Name,
						Namespace: namespace.Name,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, nvidiaBmmCluster)).To(Succeed())

		// Update status separately (status is a subresource)
		nvidiaBmmCluster.Status.Ready = true
		nvidiaBmmCluster.Status.VPCID = "9bb2d7d0-a017-4018-a212-a3d6b38e4ec9"
		nvidiaBmmCluster.Status.NetworkStatus = infrastructurev1beta1.NetworkStatus{
			SubnetIDs: map[string]string{
				"control-plane": "63e3909a-dfae-4b8e-8090-3269c5d2a2da",
			},
		}
		Expect(k8sClient.Status().Update(ctx, nvidiaBmmCluster)).To(Succeed())

		// Create CAPI Machine
		machine = &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-machine",
				Namespace: namespace.Name,
				Labels: map[string]string{
					clusterv1.ClusterNameLabel: cluster.Name,
				},
			},
			Spec: clusterv1.MachineSpec{
				ClusterName: cluster.Name,
				Bootstrap: clusterv1.Bootstrap{
					DataSecretName: ptr.To(bootstrapSecret.Name),
				},
				InfrastructureRef: clusterv1.ContractVersionedObjectReference{
					APIGroup: "infrastructure.cluster.x-k8s.io",
					Kind:     "NvidiaBMMMachine",
					Name:     "test-machine",
				},
			},
		}
		Expect(k8sClient.Create(ctx, machine)).To(Succeed())

		// Re-fetch machine to get UID assigned by API server
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(machine), machine)).To(Succeed())

		// Create NvidiaBMMMachine
		nvidiaBmmMachine = &infrastructurev1beta1.NvidiaBMMMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-machine",
				Namespace: namespace.Name,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.String(),
						Kind:       "Machine",
						Name:       machine.Name,
						UID:        machine.UID,
					},
				},
				Labels: map[string]string{
					clusterv1.ClusterNameLabel: cluster.Name,
				},
			},
			Spec: infrastructurev1beta1.NvidiaBMMMachineSpec{
				InstanceType: infrastructurev1beta1.InstanceTypeSpec{
					ID: "eaaf1d9d-7322-442e-b23f-3275d3e48198",
				},
				Network: infrastructurev1beta1.NetworkSpec{
					SubnetName: "control-plane",
				},
				SSHKeyGroups: []string{"164fa137-ef87-4352-b66c-933460d8449b"},
			},
		}
		Expect(k8sClient.Create(ctx, nvidiaBmmMachine)).To(Succeed())
	})

	AfterEach(func() {
		// Clean up namespace
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
	})

	It("should add finalizer to NvidiaBMMMachine", func() {
		Eventually(func() []string {
			updated := &infrastructurev1beta1.NvidiaBMMMachine{}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(nvidiaBmmMachine), updated)
			if err != nil {
				return nil
			}
			return updated.Finalizers
		}, 10*time.Second, 500*time.Millisecond).Should(ContainElement(controller.NvidiaBMMMachineFinalizer))
	})

	It("should wait for cluster to be ready before provisioning", func() {
		// Create machine with cluster not ready
		notReadyCluster := &infrastructurev1beta1.NvidiaBMMCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "not-ready-cluster",
				Namespace: namespace.Name,
			},
			Spec: infrastructurev1beta1.NvidiaBMMClusterSpec{
				SiteRef: infrastructurev1beta1.SiteReference{
					ID: "8a880c71-fe4b-4e43-9e24-ebfcb8a84c5f",
				},
				TenantID: "b013708a-99f0-47b2-a630-cabb4ae1d3df",
				VPC: infrastructurev1beta1.VPCSpec{
					Name:                      "not-ready-vpc",
					NetworkVirtualizationType: "ETHERNET_VIRTUALIZER",
				},
				Subnets: []infrastructurev1beta1.SubnetSpec{
					{
						Name: "default",
						CIDR: "10.150.1.0/24",
					},
				},
				Authentication: infrastructurev1beta1.AuthenticationSpec{
					SecretRef: corev1.SecretReference{
						Name:      credSecret.Name,
						Namespace: namespace.Name,
					},
				},
			},
			Status: infrastructurev1beta1.NvidiaBMMClusterStatus{
				Ready: false, // Not ready
			},
		}
		Expect(k8sClient.Create(ctx, notReadyCluster)).To(Succeed())

		capiCluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "not-ready-cluster",
				Namespace: namespace.Name,
			},
			Spec: clusterv1.ClusterSpec{
				InfrastructureRef: clusterv1.ContractVersionedObjectReference{
					APIGroup: "infrastructure.cluster.x-k8s.io",
					Kind:     "NvidiaBMMCluster",
					Name:     "not-ready-cluster",
				},
			},
		}
		Expect(k8sClient.Create(ctx, capiCluster)).To(Succeed())

		waitingMachine := &infrastructurev1beta1.NvidiaBMMMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "waiting-machine",
				Namespace: namespace.Name,
				Labels: map[string]string{
					clusterv1.ClusterNameLabel: "not-ready-cluster",
				},
			},
		}
		Expect(k8sClient.Create(ctx, waitingMachine)).To(Succeed())

		// Should not provision instance (no instance ID in status)
		Consistently(func() string {
			updated := &infrastructurev1beta1.NvidiaBMMMachine{}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(waitingMachine), updated)
			if err != nil {
				return ""
			}
			return updated.Status.InstanceID
		}, 3*time.Second, 500*time.Millisecond).Should(BeEmpty())
	})

	It("should handle deletion gracefully", func() {
		// Delete the machine
		Expect(k8sClient.Delete(ctx, nvidiaBmmMachine)).To(Succeed())

		// Should be removed eventually (after finalizer processing)
		Eventually(func() bool {
			updated := &infrastructurev1beta1.NvidiaBMMMachine{}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(nvidiaBmmMachine), updated)
			return err != nil
		}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
	})
})
