# Install Dynamic Scoring Framework on OpenShift Container Platform

## Prepare Image

### Prerequisites

Ensure that the OpenShift Internal Registry is enabled and accessible. For more information, refer to the official OpenShift documentation:

https://docs.redhat.com/en/documentation/openshift_container_platform/4.18/html/registry/securing-exposing-registry#securing-exposing-registry

### Push Image to OpenShift Internal Registry

```bash
# after login to OpenShift hub cluster
HOST=$(oc get route default-route -n openshift-image-registry --template='{{ .spec.host }}')
podman login -u ai-ran-admin -p $(oc whoami -t) $HOST
# build and push controller image
IMG_CONTROLLER=$HOST/dynamic-scoring/dynamic-scoring-controller:latest
make docker-build IMG_CONTROLLER=$IMG_CONTROLLER
podman push $IMG_CONTROLLER
# build and push addon image
IMG_ADDON=$HOST/open-cluster-management/dynamic-scoring-addon:latest
make docker-build-addon IMG_ADDON=$IMG_ADDON
podman push $IMG_ADDON
```

### (Optional) build and push arm64 images from external cluster

```
oc create sa dynamic-scoring-image-pusher -n open-cluster-management
oc policy add-role-to-user system:image-pusher -z dynamic-scoring-image-pusher -n open-cluster-management
oc get secret dynamic-scoring-image-pusher-dockercfg-xxxxx -n open-cluster-management -o jsonpath='{.data.\.dockercfg}' | base64 -d > hub-dockerconfig.json

```

```bash
oc create secret generic hub-push-secret \
  --from-file=.dockercfg=./hub-dockerconfig.json \
  --type=kubernetes.io/dockercfg
```

```
# before running these commands, make sure you have push secret build-push-secret in managed cluster
oc apply -f hack/image-build/buildconfig-addon-aarch64.yaml
oc start-build addon-aarch64-build --from-dir=. --follow
```

## Deploy Dynamic Scoring Framework

```bash
# after login to OpenShift cluster
make deploy IMG_CONTROLLER=$IMG_CONTROLLER
make deploy-addon IMG_ADDON=$IMG_ADDON
```

## (Optional) Configure Agent image pull from Hub registry

If you push the addon image to the hub cluster's internal registry (e.g. `$HOST/open-cluster-management/dynamic-scoring-addon:latest`), the managed clusters also need credentials to pull it.

This addon supports overriding the agent image and imagePullSecrets via `AddOnDeploymentConfig`.

### 1) Create a pull secret on each managed cluster

Create a `kubernetes.io/dockerconfigjson` secret in the agent install namespace (default: `dynamic-scoring`) on each managed cluster.

Example (run on the managed cluster):

```bash
NAMESPACE=open-cluster-management
SECRET_NAME=dynamic-scoring-addon-pull-secret
HOST=$(oc get route default-route -n openshift-image-registry --template='{{ .spec.host }}')

# Login once on your workstation (hub), then create the secret on managed clusters.
podman login -u ai-ran-admin -p $(oc whoami -t) $HOST

# Create secret using the same credentials
oc -n $NAMESPACE create secret docker-registry $SECRET_NAME \
	--docker-server=$HOST \
	--docker-username=ai-ran-admin \
	--docker-password=$(oc whoami -t)
```

### 2) Update `AddOnDeploymentConfig` to use the secret

Edit the `AddOnDeploymentConfig` referenced by `ClusterManagementAddOn` (this repo uses `name: dynamic-scoring-addon-config` in namespace `open-cluster-management`).

```yaml
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: AddOnDeploymentConfig
metadata:
	name: dynamic-scoring-addon-config
	namespace: open-cluster-management
spec:
	agentInstallNamespace: dynamic-scoring
	customizedVariables:
		# override agent image
		- name: Image
			value: your-image-registry/open-cluster-management/dynamic-scoring-addon:latest

		# set imagePullSecrets for managed cluster agent
		- name: ImagePullSecrets
			value: '["hub-registry-secret"]'
```

Note: `ImagePullSecrets` is interpreted as an array of secret names.
