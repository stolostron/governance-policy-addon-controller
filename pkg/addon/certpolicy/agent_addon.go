package certpolicy

import (
	"context"
	"embed"
	"fmt"
	"os"
	"strconv"

	"github.com/openshift/library-go/pkg/controller/controllercmd"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	"open-cluster-management.io/addon-framework/pkg/addonmanager"
	"open-cluster-management.io/addon-framework/pkg/agent"
	"open-cluster-management.io/addon-framework/pkg/utils"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	addonv1alpha1client "open-cluster-management.io/api/client/addon/clientset/versioned"
	clusterv1client "open-cluster-management.io/api/client/cluster/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	policyaddon "open-cluster-management.io/governance-policy-addon-controller/pkg/addon"
)

const (
	addonName = "cert-policy-controller"
)

var log = ctrl.Log.WithName("certpolicy")

type UserValues struct {
	GlobalValues                  policyaddon.GlobalValues `json:"global,"`
	KubernetesDistribution        string                   `json:"kubernetesDistribution"`
	HostingKubernetesDistribution string                   `json:"hostingKubernetesDistribution"`
	Prometheus                    map[string]interface{}   `json:"prometheus"`
	UserArgs                      policyaddon.UserArgs     `json:"args,"`
}

// FS go:embed
//
//go:embed manifests
//go:embed manifests/managedclusterchart
//go:embed manifests/managedclusterchart/templates/_helpers.tpl
var FS embed.FS

var agentPermissionFiles = []string{
	// role with RBAC rules to access resources on hub
	"manifests/hubpermissions/role.yaml",
	// rolebinding to bind the above role to a certain user group
	"manifests/hubpermissions/rolebinding.yaml",
}

func getValues(
	ctx context.Context,
	clusterClient *clusterv1client.Clientset,
) func(*clusterv1.ManagedCluster, *addonapiv1alpha1.ManagedClusterAddOn) (addonfactory.Values, error) {
	return func(
		cluster *clusterv1.ManagedCluster, addon *addonapiv1alpha1.ManagedClusterAddOn,
	) (addonfactory.Values, error) {
		userValues := UserValues{
			GlobalValues: policyaddon.GlobalValues{
				ImagePullPolicy: "IfNotPresent",
				ImagePullSecret: "open-cluster-management-image-pull-credentials",
				ImageOverrides: map[string]string{
					"cert_policy_controller": os.Getenv("CERT_POLICY_CONTROLLER_IMAGE"),
				},
				ProxyConfig: map[string]string{
					"HTTP_PROXY":  "",
					"HTTPS_PROXY": "",
					"NO_PROXY":    "",
				},
			},
			Prometheus: map[string]interface{}{},
			UserArgs: policyaddon.UserArgs{
				LogEncoder:  "console",
				LogLevel:    0,
				PkgLogLevel: -1,
			},
		}

		userValues.KubernetesDistribution = policyaddon.GetClusterVendor(cluster)

		hostingClusterName := addon.Annotations["addon.open-cluster-management.io/hosting-cluster-name"]
		if hostingClusterName != "" {
			hostingCluster, err := clusterClient.ClusterV1().ManagedClusters().Get(
				ctx, hostingClusterName, metav1.GetOptions{},
			)
			if err != nil {
				return nil, err
			}

			userValues.HostingKubernetesDistribution = policyaddon.GetClusterVendor(hostingCluster)
		} else {
			userValues.HostingKubernetesDistribution = userValues.KubernetesDistribution
		}

		// Enable Prometheus metrics by default on OpenShift
		userValues.Prometheus["enabled"] = userValues.HostingKubernetesDistribution == "OpenShift"

		annotations := addon.GetAnnotations()

		if val, ok := annotations[policyaddon.PrometheusEnabledAnnotation]; ok {
			valBool, err := strconv.ParseBool(val)
			if err != nil {
				log.Error(err, fmt.Sprintf(
					"Failed to verify '%s' annotation value '%s' for component %s (falling back to default value %v)",
					policyaddon.PrometheusEnabledAnnotation, val, addonName, userValues.Prometheus["enabled"]),
				)
			} else {
				userValues.Prometheus["enabled"] = valBool
			}
		}

		if val, ok := annotations[policyaddon.PolicyLogLevelAnnotation]; ok {
			logLevel := policyaddon.GetLogLevel(addonName, val)
			userValues.UserArgs.LogLevel = logLevel
			userValues.UserArgs.PkgLogLevel = logLevel - 2
		}

		return addonfactory.JsonStructToValues(userValues)
	}
}

// mandateValues sets deployment variables regardless of user overrides. As a result, caution should
// be taken when adding settings to this function.
func mandateValues(
	cluster *clusterv1.ManagedCluster,
	_ *addonapiv1alpha1.ManagedClusterAddOn,
) (addonfactory.Values, error) {
	values := addonfactory.Values{}

	// Don't allow replica overrides for older Kubernetes
	if policyaddon.IsOldKubernetes(cluster) {
		values["replicas"] = 1
	}

	return values, nil
}

func GetAgentAddon(ctx context.Context, controllerContext *controllercmd.ControllerContext) (agent.AgentAddon, error) {
	registrationOption := policyaddon.NewRegistrationOption(
		controllerContext,
		addonName,
		agentPermissionFiles,
		FS,
		false)

	addonClient, err := addonv1alpha1client.NewForConfig(controllerContext.KubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve addon client: %w", err)
	}

	clusterClient, err := policyaddon.GetManagedClusterClient(ctx, controllerContext.KubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize a managed cluster client: %w", err)
	}

	return addonfactory.NewAgentAddonFactory(addonName, FS, "manifests/managedclusterchart").
		WithConfigGVRs(utils.AddOnDeploymentConfigGVR).
		WithGetValuesFuncs(
			addonfactory.GetAddOnDeploymentConfigValues(
				addonfactory.NewAddOnDeploymentConfigGetter(addonClient),
				addonfactory.ToAddOnNodePlacementValues,
				addonfactory.ToAddOnCustomizedVariableValues,
			),
			getValues(ctx, clusterClient),
			addonfactory.GetValuesFromAddonAnnotation,
			mandateValues,
		).
		WithManagedClusterClient(clusterClient).
		WithAgentRegistrationOption(registrationOption).
		WithAgentInstallNamespace(
			policyaddon.
				CommonAgentInstallNamespaceFromDeploymentConfigFunc(utils.NewAddOnDeploymentConfigGetter(addonClient)),
		).
		WithScheme(policyaddon.Scheme).
		WithAgentHostedModeEnabledOption().
		BuildHelmAgentAddon()
}

func GetAndAddAgent(
	ctx context.Context, mgr addonmanager.AddonManager, controllerContext *controllercmd.ControllerContext,
) error {
	return policyaddon.GetAndAddAgent(ctx, mgr, addonName, controllerContext, GetAgentAddon)
}
