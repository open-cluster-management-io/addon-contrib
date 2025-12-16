import os
import time
from datetime import datetime
from kubernetes import client, config

# Initialize Kubernetes client
try:
    # Try to load in-cluster config (when running in a Pod)
    config.load_incluster_config()
except config.ConfigException:
    # Fall back to kubeconfig (for local development)
    try:
        config.load_kube_config()
    except config.ConfigException:
        # If neither works, the k8s endpoints will return errors
        pass

# Env vars
POLL_INTERVAL = int(os.getenv("POLL_INTERVAL_SEC", "30"))
STABLE_DURATION = int(os.getenv("STABLE_DURATION_SEC", "120"))
TARGET_CLAIM_NAME = os.getenv("TARGET_CLAIM_NAME", "policy-watcher-claim")
WATCH_NAMESPACE = os.getenv("WATCH_NAMESPACE", "default")

# OCM Policy CRD info
POLICY_GROUP = "policy.open-cluster-management.io"
POLICY_VERSION = "v1"
POLICY_PLURAL = "policies"

# ClusterClaim CRD info
CLAIM_GROUP = "cluster.open-cluster-management.io"
CLAIM_VERSION = "v1alpha1"
CLAIM_PLURAL = "clusterclaims"


def all_policies_compliant():
    api = client.CustomObjectsApi()
    policies = api.list_namespaced_custom_object(
        group=POLICY_GROUP,
        version=POLICY_VERSION,
        namespace=WATCH_NAMESPACE,
        plural=POLICY_PLURAL,
    )["items"]

    if not policies:
        return "empty"

    for p in policies:
        status = p.get("status", {})
        if status.get("compliant") != "Compliant":
            return "noncompliant"

    return "compliant"


def get_cluster_claim_value():
    api = client.CustomObjectsApi()
    claim = api.get_cluster_custom_object(
        group=CLAIM_GROUP,
        version=CLAIM_VERSION,
        plural=CLAIM_PLURAL,
        name=TARGET_CLAIM_NAME,
    )
    return claim.get("spec", {}).get("value")


def update_cluster_claim(value):
    api = client.CustomObjectsApi()
    body = {"spec": {"value": value}}

    api.patch_cluster_custom_object(
        group=CLAIM_GROUP,
        version=CLAIM_VERSION,
        plural=CLAIM_PLURAL,
        name=TARGET_CLAIM_NAME,
        body=body,
    )
    print(f"[INFO] ClusterClaim '{TARGET_CLAIM_NAME}' updated to '{value}'")


def main():
    stable_since = None
    print("[INFO] Starting policy compliance watcher...")

    while True:
        try:
            compliant = all_policies_compliant()
            now = datetime.now()

            current_claim = get_cluster_claim_value()

            if compliant == "compliant":
                if stable_since is None:
                    stable_since = now
                else:
                    elapsed = (now - stable_since).total_seconds()
                    if elapsed >= STABLE_DURATION:
                        if current_claim != "compliant":
                            print("[INFO] Policies stable. Updating ClusterClaim.")
                            update_cluster_claim("compliant")
            elif compliant == "noncompliant":
                stable_since = None
                if current_claim != "noncompliant":
                    update_cluster_claim("noncompliant")
            else:
                stable_since = None
                if current_claim != "empty":
                    update_cluster_claim("empty")

        except Exception as e:
            print(f"[ERROR] {e}")

        time.sleep(POLL_INTERVAL)


if __name__ == "__main__":
    main()
