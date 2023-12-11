// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

const (
	case4ManagedClusterAddOnName  string = "cert-policy-controller"
	case4ManagedClusterAddOnCR    string = "../resources/cert_policy_addon_cr.yaml"
	case4ClusterManagementAddOnCR string = "../resources/cert_policy_clustermanagementaddon.yaml"
	case4DeploymentName           string = "cert-policy-controller"
	case4PodSelector              string = "app=cert-policy-controller"
)

func verifyCertPolicyDeployment(
	logPrefix string, client dynamic.Interface, clusterName, namespace string, clusterNum int,
) {
	By(logPrefix + "checking the number of containers in the deployment")

	deploy := GetWithTimeout(
		client, gvrDeployment, case4DeploymentName, namespace, true, 60,
	)
	Expect(deploy).NotTo(BeNil())

	Eventually(func() []interface{} {
		deploy = GetWithTimeout(
			client, gvrDeployment, case4DeploymentName, namespace, true, 30,
		)
		containers, _, _ := unstructured.NestedSlice(deploy.Object, "spec", "template", "spec", "containers")

		return containers
	}, 60, 1).Should(HaveLen(1))

	if startupProbeInCluster(clusterNum) {
		By(logPrefix + "verifying all replicas in cert-policy-controller deployment are available")
		Eventually(func() bool {
			deploy = GetWithTimeout(
				client, gvrDeployment, case4DeploymentName, namespace, true, 30,
			)
			replicas, found, err := unstructured.NestedInt64(deploy.Object, "status", "replicas")
			if !found || err != nil {
				return false
			}

			available, found, err := unstructured.NestedInt64(deploy.Object, "status", "availableReplicas")
			if !found || err != nil {
				return false
			}

			return available == replicas
		}, 240, 1).Should(Equal(true))
	}

	By(logPrefix + "verifying a running cert-policy-controller pod")
	Eventually(func() string {
		opts := metav1.ListOptions{
			LabelSelector: case4PodSelector,
		}
		pods := ListWithTimeoutByNamespace(client, gvrPod, opts, namespace, 1, true, 30)
		phase, _, _ := unstructured.NestedString(pods.Items[0].Object, "status", "phase")

		return phase
	}, 60, 1).Should(Equal("Running"))

	By(logPrefix + "showing the cert-policy-controller managedclusteraddon as available")
	Eventually(func() bool {
		addon := GetWithTimeout(
			clientDynamic, gvrManagedClusterAddOn, case4DeploymentName, clusterName, true, 30,
		)

		return getAddonStatus(addon)
	}, 240, 1).Should(Equal(true))
}

var _ = Describe("Test cert-policy-controller deployment", func() {
	It("should create the default cert-policy-controller deployment on the managed cluster", func() {
		for i, cluster := range managedClusterList {
			logPrefix := cluster.clusterType + " " + cluster.clusterName + ": "
			By(logPrefix + "deploying the default cert-policy-controller managedclusteraddon")
			Kubectl("apply", "-n", cluster.clusterName, "-f", case4ManagedClusterAddOnCR)

			verifyCertPolicyDeployment(logPrefix, cluster.clusterClient, cluster.clusterName, addonNamespace, i)

			By(logPrefix + "removing the cert-policy-controller deployment when the ManagedClusterAddOn CR is removed")
			Kubectl("delete", "-n", cluster.clusterName, "-f", case4ManagedClusterAddOnCR)
			deploy := GetWithTimeout(
				cluster.clusterClient, gvrDeployment, case4DeploymentName, addonNamespace, false, 30,
			)
			Expect(deploy).To(BeNil())
		}
	})

	It("should create a cert-policy-controller deployment with node selector on the managed cluster", func() {
		By("Creating the AddOnDeploymentConfig")
		Kubectl("apply", "-f", addOnDeplomentConfigCR)
		By("Creating the cert-policy-controller ClusterManagementAddOn to use the AddOnDeploymentConfig")
		Kubectl("apply", "-f", case4ClusterManagementAddOnCR)

		for i, cluster := range managedClusterList {
			logPrefix := cluster.clusterType + " " + cluster.clusterName + ": "
			By(logPrefix + "deploying the default cert-policy-controller managedclusteraddon")
			Kubectl("apply", "-n", cluster.clusterName, "-f", case4ManagedClusterAddOnCR)

			verifyCertPolicyDeployment(logPrefix, cluster.clusterClient, cluster.clusterName, addonNamespace, i)

			By(logPrefix + "verifying the nodeSelector")
			deploy := GetWithTimeout(
				cluster.clusterClient, gvrDeployment, case4DeploymentName, addonNamespace, true, 30,
			)

			nodeSelector, _, _ := unstructured.NestedStringMap(
				deploy.Object, "spec", "template", "spec", "nodeSelector",
			)
			Expect(nodeSelector).To(Equal(map[string]string{"kubernetes.io/os": "linux"}))

			By(logPrefix + "verifying the tolerations")
			tolerations, _, _ := unstructured.NestedSlice(deploy.Object, "spec", "template", "spec", "tolerations")
			Expect(tolerations).To(HaveLen(1))
			expected := map[string]interface{}{
				"key":      "dedicated",
				"operator": "Equal",
				"value":    "something-else",
				"effect":   "NoSchedule",
			}
			Expect(tolerations[0]).To(Equal(expected))

			By(logPrefix +
				"removing the cert-policy-controller deployment when the ManagedClusterAddOn CR is removed")
			Kubectl("delete", "-n", cluster.clusterName, "-f", case4ManagedClusterAddOnCR)
			deploy = GetWithTimeout(
				cluster.clusterClient, gvrDeployment, case2DeploymentName, addonNamespace, false, 30,
			)
			Expect(deploy).To(BeNil())
		}

		By("Deleting the AddOnDeploymentConfig")
		Kubectl("delete", "-f", addOnDeplomentConfigCR)
		By("Deleting the cert-policy-controller ClusterManagementAddOn to use the AddOnDeploymentConfig")
		Kubectl("delete", "-f", case4ClusterManagementAddOnCR)
	})

	It("should create the default cert-policy-controller deployment in hosted mode", Label("hosted-mode"), func() {
		for i, cluster := range managedClusterList[1:] {
			Expect(cluster.clusterType).To(Equal("managed"))

			cluster = managedClusterConfig{
				clusterClient: cluster.clusterClient,
				clusterName:   cluster.clusterName,
				clusterType:   cluster.clusterType,
				hostedOnHub:   true,
				kubeconfig:    cluster.kubeconfig,
			}
			hubClusterConfig := managedClusterList[0]
			hubClient := hubClusterConfig.clusterClient
			installNamespace := fmt.Sprintf("%s-hosted", cluster.clusterName)
			logPrefix := cluster.clusterType + " " + cluster.clusterName + ": "

			setupClusterSecretForHostedMode(
				logPrefix, hubClient, "cert-policy-controller-managed-kubeconfig",
				string(cluster.kubeconfig), installNamespace)

			installAddonInHostedMode(
				logPrefix, hubClient, case4ManagedClusterAddOnName,
				cluster.clusterName, hubClusterConfig.clusterName, installNamespace, nil)

			// Use i+1 since the for loop ranges over a slice skipping first index
			verifyCertPolicyDeployment(logPrefix, hubClient, cluster.clusterName, installNamespace, i+1)

			By(logPrefix +
				"removing the cert-policy-controller deployment when the ManagedClusterAddOn CR is removed")
			err := hubClient.Resource(gvrSecret).Namespace(installNamespace).Delete(
				context.TODO(), "cert-policy-controller-managed-kubeconfig", metav1.DeleteOptions{},
			)
			Expect(err).ToNot(HaveOccurred())

			err = clientDynamic.Resource(gvrManagedClusterAddOn).Namespace(cluster.clusterName).Delete(
				context.TODO(), case4ManagedClusterAddOnName, metav1.DeleteOptions{},
			)
			Expect(err).ToNot(HaveOccurred())

			deploy := GetWithTimeout(
				hubClient, gvrDeployment, case4DeploymentName, installNamespace, false, 30,
			)
			Expect(deploy).To(BeNil())

			namespace := GetWithTimeout(hubClient, gvrNamespace, installNamespace, "", false, 30)
			Expect(namespace).To(BeNil())
		}
	})

	It("should create the default cert-policy-controller deployment in hosted mode in klusterlet agent namespace",
		Label("hosted-mode"), func() {
			By("Creating the AddOnDeploymentConfig")
			Kubectl("apply", "-f", addOnDeplomentConfigWithCustomVarsCR)
			By("Creating the cert-policy-controller ClusterManagementAddOn to use the AddOnDeploymentConfig")
			Kubectl("apply", "-f", case4ClusterManagementAddOnCR)

			for i, cluster := range managedClusterList[1:] {
				Expect(cluster.clusterType).To(Equal("managed"))

				cluster = managedClusterConfig{
					clusterClient: cluster.clusterClient,
					clusterName:   cluster.clusterName,
					clusterType:   cluster.clusterType,
					hostedOnHub:   true,
				}
				hubClusterConfig := managedClusterList[0]
				hubClient := hubClusterConfig.clusterClient
				installNamespace := fmt.Sprintf("klusterlet-%s", cluster.clusterName)
				logPrefix := cluster.clusterType + " " + cluster.clusterName + ": "

				setupClusterSecretForHostedMode(
					logPrefix, hubClient, "external-managed-kubeconfig",
					string(hubKubeconfigInternal), installNamespace)

				installAddonInHostedMode(
					logPrefix, hubClient, case4ManagedClusterAddOnName,
					cluster.clusterName, hubClusterConfig.clusterName, installNamespace, nil)

				// Use i+1 since the for loop ranges over a slice skipping first index
				verifyCertPolicyDeployment(logPrefix, hubClient, cluster.clusterName, installNamespace, i+1)

				By(logPrefix + "Removing the ManagedClusterAddOn CR")
				err := clientDynamic.Resource(gvrManagedClusterAddOn).Namespace(cluster.clusterName).Delete(
					context.TODO(), case4ManagedClusterAddOnName, metav1.DeleteOptions{},
				)
				Expect(err).ToNot(HaveOccurred())

				By(logPrefix +
					"Verifying controller deployment is removed when the ManagedClusterAddOn CR is removed")

				deploy := GetWithTimeout(
					hubClient, gvrDeployment, case4DeploymentName, installNamespace, false, 30,
				)
				Expect(deploy).To(BeNil())

				By(logPrefix + "Verifying install namespace is not removed when the ManagedClusterAddOn CR is removed")
				namespace := GetWithTimeout(hubClient, gvrNamespace, installNamespace, "", true, 30)
				Expect(namespace).NotTo(BeNil())

				By(logPrefix + "cleaning up  the hosting cluster secret")
				err = hubClient.Resource(gvrSecret).Namespace(installNamespace).Delete(
					context.TODO(), "external-managed-kubeconfig", metav1.DeleteOptions{},
				)
				Expect(err).ToNot(HaveOccurred())

				By(logPrefix + "Cleaning up the install namespace")
				err = hubClient.Resource(gvrNamespace).Delete(
					context.TODO(), installNamespace, metav1.DeleteOptions{},
				)
				Expect(err).ToNot(HaveOccurred())

				namespace = GetWithTimeout(hubClient, gvrNamespace, installNamespace, "", false, 30)
				Expect(namespace).To(BeNil())
			}
			By("Deleting the AddOnDeploymentConfig")
			Kubectl("delete", "-f", addOnDeplomentConfigWithCustomVarsCR)
			By("Deleting the cert-policy-controller ClusterManagementAddOn to use the AddOnDeploymentConfig")
			Kubectl("delete", "-f", case4ClusterManagementAddOnCR)
		})

	It("should create a cert-policy-controller deployment with custom logging levels", func() {
		for _, cluster := range managedClusterList {
			logPrefix := cluster.clusterType + " " + cluster.clusterName + ": "
			By(logPrefix + "deploying the default cert-policy-controller managedclusteraddon")
			Kubectl("apply", "-n", cluster.clusterName, "-f", case4ManagedClusterAddOnCR)
			deploy := GetWithTimeout(
				cluster.clusterClient, gvrDeployment, case4DeploymentName, addonNamespace, true, 30,
			)
			Expect(deploy).NotTo(BeNil())

			By(logPrefix + "showing the cert-policy-controller managedclusteraddon as available")
			Eventually(func() bool {
				addon := GetWithTimeout(
					clientDynamic, gvrManagedClusterAddOn, case4DeploymentName, cluster.clusterName, true, 30,
				)

				return getAddonStatus(addon)
			}, 240, 1).Should(Equal(true))

			By(logPrefix + "annotating the managedclusteraddon with the " + loggingLevelAnnotation + " annotation")
			Kubectl("annotate", "-n", cluster.clusterName, "-f", case4ManagedClusterAddOnCR, loggingLevelAnnotation)

			By(logPrefix + "verifying a new cert-policy-controller pod is deployed with the logging level")
			Eventually(func(g Gomega) {
				opts := metav1.ListOptions{
					LabelSelector: case4PodSelector,
				}
				pods := ListWithTimeoutByNamespace(cluster.clusterClient, gvrPod, opts, addonNamespace, 1, true, 60)
				phase := pods.Items[0].Object["status"].(map[string]interface{})["phase"]

				g.Expect(phase.(string)).To(Equal("Running"))
				containerList, _, err := unstructured.NestedSlice(pods.Items[0].Object, "spec", "containers")
				g.Expect(err).ToNot(HaveOccurred())
				for _, container := range containerList {
					containerObj, ok := container.(map[string]interface{})
					g.Expect(ok).To(BeTrue())
					if g.Expect(containerObj).To(HaveKey("name")) && containerObj["name"] != case4DeploymentName {
						continue
					}
					if g.Expect(containerObj).To(HaveKey("args")) {
						args := containerObj["args"]
						g.Expect(args).To(ContainElement("--log-encoder=console"))
						g.Expect(args).To(ContainElement("--log-level=8"))
						g.Expect(args).To(ContainElement("--v=6"))
						g.Expect(args).To(ContainElement("--leader-elect=false"))
					}
				}
			}, 180, 10).Should(Succeed())

			By(logPrefix + "removing the cert-policy-controller deployment when the ManagedClusterAddOn CR is removed")
			Kubectl("delete", "-n", cluster.clusterName, "-f", case4ManagedClusterAddOnCR)
			deploy = GetWithTimeout(
				cluster.clusterClient, gvrDeployment, case4DeploymentName, addonNamespace, false, 30,
			)
			Expect(deploy).To(BeNil())
		}
	})
})
