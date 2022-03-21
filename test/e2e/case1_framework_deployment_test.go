// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	case1ManagedClusterAddOnCR string = "../resources/framework_addon_cr.yaml"
	case1hubAnnotationMCAOCR   string = "../resources/framework_hub_annotation_addon_cr.yaml"
	case1hubValuesMCAOCR       string = "../resources/framework_hub_values_addon_cr.yaml"
	case1DeploymentName        string = "governance-policy-framework"
	case1PodSelector           string = "app=governance-policy-framework"
	case1MWName                string = "addon-governance-policy-framework-deploy"
	case1MWPatch               string = "../resources/manifestwork_add_patch.json"
)

var _ = Describe("Test framework deployment", func() {
	It("should create the default framework deployment on separate managed clusters", func() {
		for _, cluster := range managedClusterList[1:] {
			logPrefix := cluster.clusterType + " " + cluster.clusterName + ": "
			By(logPrefix + "deploying the default framework managedclusteraddon")
			Kubectl("apply", "-n", cluster.clusterName, "-f", case1ManagedClusterAddOnCR)
			deploy := GetWithTimeout(
				cluster.clusterClient, gvrDeployment, case1DeploymentName, addonNamespace, true, 30,
			)
			Expect(deploy).NotTo(BeNil())

			checkContainersAndAvailability(cluster)

			By(logPrefix + "removing the framework deployment when the ManagedClusterAddOn CR is removed")
			Kubectl("delete", "-n", cluster.clusterName, "-f", case1ManagedClusterAddOnCR)
			deploy = GetWithTimeout(
				cluster.clusterClient, gvrDeployment, case1DeploymentName, addonNamespace, false, 30,
			)
			Expect(deploy).To(BeNil())
		}
	})

	It("should create a framework deployment with custom logging levels", func() {
		for _, cluster := range managedClusterList {
			logPrefix := cluster.clusterType + " " + cluster.clusterName + ": "
			By(logPrefix + "deploying the default framework managedclusteraddon")
			if cluster.clusterType == "hub" {
				Kubectl("apply", "-n", cluster.clusterName, "-f", case1hubAnnotationMCAOCR)
			} else {
				Kubectl("apply", "-n", cluster.clusterName, "-f", case1ManagedClusterAddOnCR)
			}

			deploy := GetWithTimeout(
				cluster.clusterClient, gvrDeployment, case1DeploymentName, addonNamespace, true, 30,
			)
			Expect(deploy).NotTo(BeNil())

			checkContainersAndAvailability(cluster)

			By(logPrefix + "annotating the managedclusteraddon with the " + loggingLevelAnnotation + " annotation")
			Kubectl("annotate", "-n", cluster.clusterName, "-f", case1ManagedClusterAddOnCR, loggingLevelAnnotation)

			checkArgs(cluster, "--log-encoder=console", "--log-level=8", "--v=6")

			By(logPrefix + "removing the framework deployment when the ManagedClusterAddOn CR is removed")
			Kubectl("delete", "-n", cluster.clusterName, "-f", case1ManagedClusterAddOnCR)
			deploy = GetWithTimeout(
				cluster.clusterClient, gvrDeployment, case1DeploymentName, addonNamespace, false, 30,
			)
			Expect(deploy).To(BeNil())
		}
	})

	It("should deploy with 2 containers if onManagedClusterHub is set in helm values annotation", func() {
		cluster := managedClusterList[0]
		Expect(cluster.clusterType).To(Equal("hub"))

		logPrefix := cluster.clusterType + " " + cluster.clusterName + ": "

		By(logPrefix + "deploying the default framework managedclusteraddon")
		Kubectl("apply", "-n", cluster.clusterName, "-f", case1hubValuesMCAOCR)
		deploy := GetWithTimeout(
			cluster.clusterClient, gvrDeployment, case1DeploymentName, addonNamespace, true, 30,
		)
		Expect(deploy).NotTo(BeNil())

		checkContainersAndAvailability(cluster)

		By(logPrefix + "annotating the managedclusteraddon with the " + loggingLevelAnnotation + " annotation")
		Kubectl("annotate", "-n", cluster.clusterName, "-f", case1ManagedClusterAddOnCR, loggingLevelAnnotation)

		checkArgs(cluster, "--log-encoder=console", "--log-level=8", "--v=6")

		By(logPrefix + "deleting the managedclusteraddon")
		Kubectl("delete", "-n", cluster.clusterName, "-f", case1ManagedClusterAddOnCR)
		deploy = GetWithTimeout(
			cluster.clusterClient, gvrDeployment, case1DeploymentName, addonNamespace, false, 30,
		)
		Expect(deploy).To(BeNil())
	})

	It("should deploy with 2 containers if onManagedClusterHub is set in the custom annotation", func() {
		cluster := managedClusterList[0]
		Expect(cluster.clusterType).To(Equal("hub"))

		logPrefix := cluster.clusterType + " " + cluster.clusterName + ": "

		By(logPrefix + "deploying the default framework managedclusteraddon")
		Kubectl("apply", "-n", cluster.clusterName, "-f", case1hubAnnotationMCAOCR)
		deploy := GetWithTimeout(
			cluster.clusterClient, gvrDeployment, case1DeploymentName, addonNamespace, true, 30,
		)
		Expect(deploy).NotTo(BeNil())

		By(logPrefix + "annotating the framework managedclusteraddon with custom annotation")
		Kubectl("annotate", "-n", cluster.clusterName, "-f", case1ManagedClusterAddOnCR,
			"addon.open-cluster-management.io/on-multicluster-hub=true")

		checkContainersAndAvailability(cluster)

		checkArgs(cluster, "--log-encoder=console", "--log-level=8", "--v=6")

		By(logPrefix + "deleting the managedclusteraddon")
		Kubectl("delete", "-n", cluster.clusterName, "-f", case1ManagedClusterAddOnCR)
		deploy = GetWithTimeout(
			cluster.clusterClient, gvrDeployment, case1DeploymentName, addonNamespace, false, 30,
		)
		Expect(deploy).To(BeNil())
	})

	It("should revert edits to the ManifestWork by default", func() {
		for _, cluster := range managedClusterList {
			logPrefix := cluster.clusterType + " " + cluster.clusterName + ": "
			By(logPrefix + "deploying the default framework managedclusteraddon")
			if cluster.clusterType == "hub" {
				Kubectl("apply", "-n", cluster.clusterName, "-f", case1hubAnnotationMCAOCR)
			} else {
				Kubectl("apply", "-n", cluster.clusterName, "-f", case1ManagedClusterAddOnCR)
			}
			deploy := GetWithTimeout(
				cluster.clusterClient, gvrDeployment, case1DeploymentName, addonNamespace, true, 30,
			)
			Expect(deploy).NotTo(BeNil())

			By(logPrefix + "getting the default number of items in the ManifestWork")
			defaultLength := 0
			Eventually(func() int {
				mw := GetWithTimeout(clientDynamic, gvrManifestWork, case1MWName, cluster.clusterName, true, 15)
				manifests, _, _ := unstructured.NestedSlice(mw.Object, "spec", "workload", "manifests")
				defaultLength = len(manifests)

				return defaultLength
			}, 60, 5).ShouldNot(Equal(0))

			By(logPrefix + "patching the ManifestWork to add an item")
			Kubectl("patch", "-n", cluster.clusterName, "manifestwork", case1MWName, "--type=json",
				"--patch-file="+case1MWPatch)

			By(logPrefix + "verifying the edit is reverted")
			Eventually(func() int {
				mw := GetWithTimeout(clientDynamic, gvrManifestWork, case1MWName, cluster.clusterName, true, 15)
				manifests, _, _ := unstructured.NestedSlice(mw.Object, "spec", "workload", "manifests")

				return len(manifests)
			}, 60, 5).Should(Equal(defaultLength))

			By(logPrefix + "deleting the managedclusteraddon")
			Kubectl("delete", "-n", cluster.clusterName, "-f", case1ManagedClusterAddOnCR)
			deploy = GetWithTimeout(
				cluster.clusterClient, gvrDeployment, case1DeploymentName, addonNamespace, false, 30,
			)
			Expect(deploy).To(BeNil())
		}
	})
	It("should preserve edits to the ManifestWork if paused by annotation", func() {
		for _, cluster := range managedClusterList {
			logPrefix := cluster.clusterType + " " + cluster.clusterName + ": "
			By(logPrefix + "deploying the default framework managedclusteraddon")
			if cluster.clusterType == "hub" {
				Kubectl("apply", "-n", cluster.clusterName, "-f", case1hubAnnotationMCAOCR)
			} else {
				Kubectl("apply", "-n", cluster.clusterName, "-f", case1ManagedClusterAddOnCR)
			}
			deploy := GetWithTimeout(
				cluster.clusterClient, gvrDeployment, case1DeploymentName, addonNamespace, true, 30,
			)
			Expect(deploy).NotTo(BeNil())

			By(logPrefix + "annotating the managedclusteraddon with the pause annotation")
			Kubectl("annotate", "-n", cluster.clusterName, "-f", case1ManagedClusterAddOnCR, "policy-addon-pause=true")

			By(logPrefix + "getting the default number of items in the ManifestWork")
			defaultLength := 0
			Eventually(func() int {
				mw := GetWithTimeout(clientDynamic, gvrManifestWork, case1MWName, cluster.clusterName, true, 15)
				manifests, _, _ := unstructured.NestedSlice(mw.Object, "spec", "workload", "manifests")
				defaultLength = len(manifests)

				return defaultLength
			}, 60, 5).ShouldNot(Equal(0))

			By(logPrefix + "patching the ManifestWork to add an item")
			Kubectl("patch", "-n", cluster.clusterName, "manifestwork", case1MWName, "--type=json",
				"--patch-file="+case1MWPatch)

			By(logPrefix + "verifying the edit is not reverted")
			Consistently(func() int {
				mw := GetWithTimeout(clientDynamic, gvrManifestWork, case1MWName, cluster.clusterName, true, 15)
				manifests, _, _ := unstructured.NestedSlice(mw.Object, "spec", "workload", "manifests")

				return len(manifests)
			}, 30, 5).Should(Equal(defaultLength + 1))

			By(logPrefix + "deleting the managedclusteraddon")
			Kubectl("delete", "-n", cluster.clusterName, "-f", case1ManagedClusterAddOnCR)
			deploy = GetWithTimeout(
				cluster.clusterClient, gvrDeployment, case1DeploymentName, addonNamespace, false, 30,
			)
			Expect(deploy).To(BeNil())
		}
	})
})

func checkContainersAndAvailability(cluster managedClusterConfig) {
	logPrefix := cluster.clusterType + " " + cluster.clusterName + ": "

	desiredContainerCount := 3
	if cluster.clusterType == "hub" {
		desiredContainerCount = 2
	}

	By(logPrefix + "checking the number of containers in the deployment")
	Eventually(func() int {
		deploy := GetWithTimeout(cluster.clusterClient, gvrDeployment,
			case1DeploymentName, addonNamespace, true, 30)
		spec := deploy.Object["spec"].(map[string]interface{})["template"].(map[string]interface{})["spec"]
		containers := spec.(map[string]interface{})["containers"]

		return len(containers.([]interface{}))
	}, 60, 1).Should(Equal(desiredContainerCount))

	By(logPrefix + "verifying all replicas in framework deployment are available")
	Eventually(func() bool {
		deploy := GetWithTimeout(
			cluster.clusterClient, gvrDeployment, case1DeploymentName, addonNamespace, true, 30,
		)
		status := deploy.Object["status"]
		replicas := status.(map[string]interface{})["replicas"]
		availableReplicas := status.(map[string]interface{})["availableReplicas"]

		return (availableReplicas != nil) && replicas.(int64) == availableReplicas.(int64)
	}, 240, 1).Should(Equal(true))

	By(logPrefix + "verifying one framework pod is running")
	Eventually(func() bool {
		opts := metav1.ListOptions{
			LabelSelector: case1PodSelector,
		}
		pods := ListWithTimeoutByNamespace(cluster.clusterClient, gvrPod, opts, addonNamespace, 1, true, 30)
		phase := pods.Items[0].Object["status"].(map[string]interface{})["phase"]

		return phase.(string) == "Running"
	}, 60, 1).Should(Equal(true))

	By(logPrefix + "showing the framework managedclusteraddon as available")
	Eventually(func() bool {
		addon := GetWithTimeout(
			clientDynamic, gvrManagedClusterAddOn, case1DeploymentName, cluster.clusterName, true, 30,
		)

		return getAddonStatus(addon)
	}, 240, 1).Should(Equal(true))
}

func checkArgs(cluster managedClusterConfig, desiredArgs ...string) {
	logPrefix := cluster.clusterType + " " + cluster.clusterName + ": "

	By(logPrefix + "verifying one framework pod is running and has the desired args")
	Eventually(func() error {
		opts := metav1.ListOptions{
			LabelSelector: case1PodSelector,
		}
		pods := ListWithTimeoutByNamespace(cluster.clusterClient, gvrPod, opts, addonNamespace, 1, true, 30)
		podObj := pods.Items[0].Object

		phase, found, err := unstructured.NestedString(podObj, "status", "phase")
		if err != nil || !found || phase != "Running" {
			return fmt.Errorf("pod phase is not running; found=%v; err=%v", found, err)
		}

		containerList, found, err := unstructured.NestedSlice(podObj, "spec", "containers")
		if err != nil || !found {
			return fmt.Errorf("could not get container list; found=%v; err=%v", found, err)
		}

		for _, container := range containerList {
			containerObj := container.(map[string]interface{})

			argList, found, err := unstructured.NestedStringSlice(containerObj, "args")
			if err != nil || !found {
				return fmt.Errorf("could not get container args; found=%v; err=%v", found, err)
			}

			Expect(argList).To(ContainElements(desiredArgs))
		}

		return nil
	}, 120, 1).Should(BeNil())
}
