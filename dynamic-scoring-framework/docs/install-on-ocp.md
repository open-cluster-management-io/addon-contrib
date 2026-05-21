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
make docker-build-controller IMG_CONTROLLER=$IMG_CONTROLLER
podman push $IMG_CONTROLLER
# build and push addon image
IMG_ADDON=$HOST/open-cluster-management/dynamic-scoring-addon:latest
make docker-build-addon IMG_ADDON=$IMG_ADDON
podman push $IMG_ADDON
```

### (Optional) build and push arm64 images from external cluster

If your environment requires arm64 images, you can build and push them from an external cluster using the following steps.
(As future work, it planned to support multi-arch builds. this step may not be needed then.)

```bash
SERVICEACCOUNT=dynamic-scoring-image-pusher
oc create sa $SERVICEACCOUNT -n open-cluster-management
oc policy add-role-to-user system:image-pusher -z $SERVICEACCOUNT -n open-cluster-management
TOKEN=$(oc create token $SERVICEACCOUNT -n open-cluster-management)
REGISTRY=$(oc get route default-route -n openshift-image-registry --template='{{ .spec.host }}')
AUTH=$(echo -n "$SERVICEACCOUNT:$TOKEN" | base64 -w0)
cat <<EOF > ./secrets/push-dockerconfig.json
{
	"auths": {
		"$REGISTRY": {
			"auth": "$AUTH"
		}
	}
}
EOF
```

```bash
# before running these commands, make sure you have push secret build-push-secret in managed cluster
oc create secret generic hub-push-secret \
  --from-file=.dockerconfigjson=./secrets/push-dockerconfig.json \
  --type=kubernetes.io/dockerconfigjson -n dynamic-scoring
oc apply -f hack/image-build/buildconfig-addon-aarch64.yaml -n dynamic-scoring
# update buildconfig for addon image
oc start-build addon-aarch64-build -n dynamic-scoring --from-dir=. --follow
# update buildconfig for policy-watcher image if needed
oc start-build policy-watcher-aarch64-build -n dynamic-scoring --from-dir=samples/policy-watcher --follow
```

NOTE: If you want to push images to hub cluster from an external cluster, make sure the external cluster has network access to the hub cluster's internal registry and existence of imagestreams.

## Deploy Dynamic Scoring Framework

```bash
# after login to OpenShift cluster
make deploy IMG_CONTROLLER=$IMG_CONTROLLER
make deploy-addon IMG_ADDON=$IMG_ADDON
```

## (Optional) Configure Agent image pull from Hub registry

If you push the addon image to the hub cluster's internal registry (e.g. `$HOST/open-cluster-management/dynamic-scoring-addon:latest`), the managed clusters also need credentials to pull it.

As default, this framework copy pull secret named `dynamic-scoring-addon-pull-secret` from namespace `open-cluster-management` on hub cluster to `hub-registry-secret` the agent install namespace (e.g. `dynamic-scoring`).

### 1) Create pull secret on Hub cluster

Following are the steps to create the pull secret on hub cluster.

```bash
SERVICEACCOUNT=dynamic-scoring-image-puller
oc create sa $SERVICEACCOUNT -n open-cluster-management
oc policy add-role-to-user system:image-puller -z $SERVICEACCOUNT -n open-cluster-management
TOKEN=$(oc create token $SERVICEACCOUNT -n open-cluster-management)
REGISTRY=default-route-openshift-image-registry.apps.hubdev01.airan.localdomain
AUTH=$(echo -n "$SERVICEACCOUNT:$TOKEN" | base64 -w0)

cat <<EOF > ./secrets/pull-dockerconfig.json
{
  "auths": {
    "$REGISTRY": {
      "auth": "$AUTH"
    }
  }
}
EOF

oc create secret generic dynamic-scoring-addon-pull-secret \
  --from-file=.dockerconfigjson=./secrets/pull-dockerconfig.json \
  --type=kubernetes.io/dockerconfigjson -n open-cluster-management
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
			value: default-route-openshift-image-registry.apps.hubdev01.airan.localdomain/open-cluster-management/dynamic-scoring-addon:latest

		# set imagePullSecrets for managed cluster agent
		- name: ImagePullSecrets
			value: '["hub-registry-secret"]'
```

Note: `ImagePullSecrets` is interpreted as an array of secret names.

oc create secret generic policy-watcher-pull-secret \
  --from-file=.dockerconfigjson=./secrets/pull-dockerconfig.json \
  --type=kubernetes.io/dockerconfigjson -n dynamic-scoring