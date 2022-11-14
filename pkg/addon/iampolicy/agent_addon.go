package iampolicy

import (
	"embed"
	"fmt"
	"os"

	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	"open-cluster-management.io/addon-framework/pkg/addonmanager"
	"open-cluster-management.io/addon-framework/pkg/agent"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	addonv1alpha1client "open-cluster-management.io/api/client/addon/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	policyaddon "open-cluster-management.io/governance-policy-addon-controller/pkg/addon"
)

const (
	addonName = "iam-policy-controller"
)

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

func getValues(cluster *clusterv1.ManagedCluster,
	addon *addonapiv1alpha1.ManagedClusterAddOn,
) (addonfactory.Values, error) {
	userValues := policyaddon.UserValues{
		GlobalValues: policyaddon.GlobalValues{
			ImagePullPolicy: "IfNotPresent",
			ImagePullSecret: "open-cluster-management-image-pull-credentials",
			ImageOverrides: map[string]string{
				"iam_policy_controller": os.Getenv("IAM_POLICY_CONTROLLER_IMAGE"),
			},
			ProxyConfig: map[string]string{
				"HTTP_PROXY":  "",
				"HTTPS_PROXY": "",
				"NO_PROXY":    "",
			},
		},
		UserArgs: policyaddon.UserArgs{
			LogEncoder:  "console",
			LogLevel:    0,
			PkgLogLevel: -1,
		},
	}

	if val, ok := addon.GetAnnotations()[policyaddon.PolicyLogLevelAnnotation]; ok {
		logLevel := policyaddon.GetLogLevel(addonName, val)
		userValues.UserArgs.LogLevel = logLevel
		userValues.UserArgs.PkgLogLevel = logLevel - 2
	}

	return addonfactory.JsonStructToValues(userValues)
}

func GetAgentAddon(controllerContext *controllercmd.ControllerContext) (agent.AgentAddon, error) {
	registrationOption := policyaddon.NewRegistrationOption(
		controllerContext,
		addonName,
		agentPermissionFiles,
		FS)

	addonClient, err := addonv1alpha1client.NewForConfig(controllerContext.KubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve addon client: %w", err)
	}

	return addonfactory.NewAgentAddonFactory(addonName, FS, "manifests/managedclusterchart").
		WithConfigGVRs(addonfactory.AddOnDeploymentConfigGVR).
		WithGetValuesFuncs(
			addonfactory.GetAddOnDeloymentConfigValues(
				addonfactory.NewAddOnDeloymentConfigGetter(addonClient), addonfactory.ToAddOnNodePlacementValues,
			),
			getValues,
			addonfactory.GetValuesFromAddonAnnotation,
		).
		WithAgentRegistrationOption(registrationOption).
		WithAgentHostedModeEnabledOption().
		BuildHelmAgentAddon()
}

func GetAndAddAgent(mgr addonmanager.AddonManager, controllerContext *controllercmd.ControllerContext) error {
	return policyaddon.GetAndAddAgent(mgr, addonName, controllerContext, GetAgentAddon)
}
