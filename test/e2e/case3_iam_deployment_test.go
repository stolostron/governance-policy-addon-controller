// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

const (
	case3ManagedClusterAddOnCR string = "../resources/iam_policy_addon_cr.yaml"
	case3DeploymentName        string = "iam-policy-controller"
	case3PodSelector           string = "app=iam-policy-controller"
)

func verifyIamPolicyDeployment(
	logPrefix string, client dynamic.Interface, clusterName, namespace string, clusterNum int,
) {
	By(logPrefix + "Checking the number of containers in the deployment")

	deploy := GetWithTimeout(
		client, gvrDeployment, case3DeploymentName, namespace, true, 30,
	)
	Expect(deploy).NotTo(BeNil())

	Eventually(func() int {
		deploy = GetWithTimeout(
			client, gvrDeployment, case3DeploymentName, namespace, true, 30,
		)
		spec := deploy.Object["spec"].(map[string]interface{})["template"].(map[string]interface{})["spec"]
		containers := spec.(map[string]interface{})["containers"]

		return len(containers.([]interface{}))
	}, 60, 1).Should(Equal(1))

	if startupProbeInCluster(clusterNum) {
		By(logPrefix + "Verifying all replicas in iam-policy-controller deployment are available")
		Eventually(func() bool {
			deploy = GetWithTimeout(
				client, gvrDeployment, case3DeploymentName, namespace, true, 30,
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

	By(logPrefix + "Verifying a running iam-policy-controller pod")
	Eventually(func() bool {
		opts := metav1.ListOptions{
			LabelSelector: case3PodSelector,
		}
		pods := ListWithTimeoutByNamespace(client, gvrPod, opts, namespace, 1, true, 30)
		phase, _, _ := unstructured.NestedString(pods.Items[0].Object, "status", "phase")

		return phase == "Running"
	}, 60, 1).Should(Equal(true))

	By(logPrefix + "Showing the iam-policy-controller managedclusteraddon as available")
	Eventually(func() bool {
		addon := GetWithTimeout(
			clientDynamic, gvrManagedClusterAddOn, case3DeploymentName, clusterName, true, 30,
		)

		return getAddonStatus(addon)
	}, 240, 1).Should(Equal(true))
}

var _ = Describe("Test iam-policy-controller deployment", func() {
	It("should create the iam-policy-controller deployment on the managed cluster", func() {
		for i, cluster := range managedClusterList {
			logPrefix := cluster.clusterType + " " + cluster.clusterName + ": "
			By(logPrefix + "deploying the default iam-policy-controller managedclusteraddon")
			Kubectl("apply", "-n", cluster.clusterName, "-f", case3ManagedClusterAddOnCR)

			verifyIamPolicyDeployment(logPrefix, cluster.clusterClient, cluster.clusterName, addonNamespace, i)

			By(logPrefix + "removing the iam-policy-controller deployment when the ManagedClusterAddOn CR is removed")
			Kubectl("delete", "-n", cluster.clusterName, "-f", case3ManagedClusterAddOnCR)
			deploy := GetWithTimeout(
				cluster.clusterClient, gvrDeployment, case3DeploymentName, addonNamespace, false, 30,
			)
			Expect(deploy).To(BeNil())
		}
	})

	It("should create the default iam-policy-controller deployment in hosted mode", Label("hosted-mode"), func() {
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
			installNamespace := fmt.Sprintf("%s-hosted", cluster.clusterName)
			logPrefix := cluster.clusterType + " " + cluster.clusterName + ": "

			By(logPrefix + "creating the install Namespace")
			installNamespaceObject := unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]interface{}{
					"name": installNamespace,
				},
			}}
			_, err := hubClient.Resource(gvrNamespace).Create(
				context.TODO(), &installNamespaceObject, metav1.CreateOptions{},
			)
			if !errors.IsAlreadyExists(err) {
				Expect(err).To(BeNil())
			}

			By(logPrefix + "creating the iam-policy-controller-managed-kubeconfig secret in install Namespace")
			secret := unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"name": "iam-policy-controller-managed-kubeconfig",
				},
				"stringData": map[string]interface{}{
					"kubeconfig": string(hubKubeconfigInternal),
				},
			}}
			_, err = hubClient.Resource(gvrSecret).Namespace(installNamespace).Create(
				context.TODO(), &secret, metav1.CreateOptions{},
			)
			Expect(err).To(BeNil())

			By(logPrefix + "deploying the default iam-policy-controller ManagedClusterAddOn in hosted mode")
			addon := unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "addon.open-cluster-management.io/v1alpha1",
				"kind":       "ManagedClusterAddOn",
				"metadata": map[string]interface{}{
					"name": "iam-policy-controller",
					"annotations": map[string]interface{}{
						"addon.open-cluster-management.io/hosting-cluster-name": managedClusterList[0].clusterName,
					},
				},
				"spec": map[string]interface{}{
					"installNamespace": installNamespace,
				},
			}}
			_, err = hubClient.Resource(gvrManagedClusterAddOn).Namespace(cluster.clusterName).Create(
				context.TODO(), &addon, metav1.CreateOptions{},
			)
			Expect(err).To(BeNil())

			verifyIamPolicyDeployment(logPrefix, hubClient, cluster.clusterName, installNamespace, i)

			By(logPrefix +
				"removing the iam-policy-controller deployment when the ManagedClusterAddOn CR is removed")
			err = hubClient.Resource(gvrSecret).Namespace(installNamespace).Delete(
				context.TODO(), secret.GetName(), metav1.DeleteOptions{},
			)
			Expect(err).To(BeNil())

			err = clientDynamic.Resource(gvrManagedClusterAddOn).Namespace(cluster.clusterName).Delete(
				context.TODO(), addon.GetName(), metav1.DeleteOptions{},
			)
			Expect(err).To(BeNil())

			deploy := GetWithTimeout(
				hubClient, gvrDeployment, case3DeploymentName, installNamespace, false, 30,
			)
			Expect(deploy).To(BeNil())

			namespace := GetWithTimeout(hubClient, gvrNamespace, installNamespace, "", false, 30)
			Expect(namespace).To(BeNil())
		}
	})

	It("should create an iam-policy-controller deployment with custom logging levels", func() {
		for _, cluster := range managedClusterList {
			logPrefix := cluster.clusterType + " " + cluster.clusterName + ": "
			By(logPrefix + "deploying the default iam-policy-controller managedclusteraddon")
			Kubectl("apply", "-n", cluster.clusterName, "-f", case3ManagedClusterAddOnCR)
			deploy := GetWithTimeout(
				cluster.clusterClient, gvrDeployment, case3DeploymentName, addonNamespace, true, 30,
			)
			Expect(deploy).NotTo(BeNil())

			By(logPrefix + "showing the iam-policy-controller managedclusteraddon as available")
			Eventually(func() bool {
				addon := GetWithTimeout(
					clientDynamic, gvrManagedClusterAddOn, case3DeploymentName, cluster.clusterName, true, 30,
				)

				return getAddonStatus(addon)
			}, 240, 1).Should(Equal(true))

			By(logPrefix + "annotating the managedclusteraddon with the " + loggingLevelAnnotation + " annotation")
			Kubectl("annotate", "-n", cluster.clusterName, "-f", case3ManagedClusterAddOnCR, loggingLevelAnnotation)

			By(logPrefix + "verifying the pod has been deployed with a new logging level")
			Eventually(func(g Gomega) {
				opts := metav1.ListOptions{
					LabelSelector: case3PodSelector,
				}
				pods := ListWithTimeoutByNamespace(cluster.clusterClient, gvrPod, opts, addonNamespace, 1, true, 60)
				phase := pods.Items[0].Object["status"].(map[string]interface{})["phase"]

				g.Expect(phase.(string)).To(Equal("Running"))
				containerList, _, err := unstructured.NestedSlice(pods.Items[0].Object, "spec", "containers")
				g.Expect(err).To(BeNil())
				for _, container := range containerList {
					containerObj, ok := container.(map[string]interface{})
					g.Expect(ok).To(BeTrue())
					if g.Expect(containerObj).To(HaveKey("name")) && containerObj["name"] != case2DeploymentName {
						continue
					}
					if g.Expect(containerObj).To(HaveKey("args")) {
						args := containerObj["args"]
						g.Expect(args).To(ContainElement("--log-encoder=console"))
						g.Expect(args).To(ContainElement("--log-level=8"))
						g.Expect(args).To(ContainElement("--v=6"))
					}
				}
			}, 180, 10).Should(Succeed())

			By(logPrefix + "removing the iam-policy-controller deployment when the ManagedClusterAddOn CR is removed")
			Kubectl("delete", "-n", cluster.clusterName, "-f", case3ManagedClusterAddOnCR)
			deploy = GetWithTimeout(
				cluster.clusterClient, gvrDeployment, case3DeploymentName, addonNamespace, false, 30,
			)
			Expect(deploy).To(BeNil())
		}
	})
})
