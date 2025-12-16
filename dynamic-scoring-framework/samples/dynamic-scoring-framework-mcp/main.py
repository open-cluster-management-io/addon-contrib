from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field
from typing import Optional, List, Dict, Any
from fastapi_mcp import FastApiMCP
from kubernetes import client, config
import pulp
import yaml
import secrets
import jinja2
import re
from pathlib import Path


class GetSampleResponse(BaseModel):
    message: str = Field(..., description="A sample response message")


class GetSampleRequest(BaseModel):
    sample_name: Optional[str] = Field(None, description="An optional sample name")


class AddonPlacementScoreItem(BaseModel):
    """A single AddonPlacementScore resource"""

    model_config = {"extra": "allow"}  # Allow additional fields (Pydantic v2)

    apiVersion: Optional[str] = Field(None, description="API version")
    kind: Optional[str] = Field(None, description="Resource kind")
    metadata: Optional[Dict[str, Any]] = Field(None, description="Resource metadata")
    status: Optional[Dict[str, Any]] = Field(None, description="Resource status")


class AddonPlacementScoreResponse(BaseModel):
    """Response containing list of AddonPlacementScores"""

    items: List[AddonPlacementScoreItem] = Field(
        default_factory=list, description="List of AddonPlacementScores"
    )


class OptimizationParams(BaseModel):
    """Request body for optimization endpoint"""

    namespace: str = Field(..., description="Target namespace")
    clusters: List[Dict[str, Any]] = Field(
        ..., description="List of cluster configurations"
    )
    targetWorkloads: List[Dict[str, Any]] = Field(
        ..., description="List of target workloads"
    )
    preference: Dict[str, Any] = Field(..., description="Preference configuration")


class OptimizationResponse(BaseModel):
    """Response containing optimization results"""

    status: str = Field(..., description="Optimization status")
    objective_value: Optional[float] = Field(
        None, description="Objective function value"
    )
    output_yaml: str = Field(..., description="Generated YAML configuration")


# --- Helper Functions for File Reading ---


def parse_with_format(fmt: str, s: str) -> Optional[Dict[str, str]]:
    """Parse string with format pattern"""
    # ${name} を (?P<name>.+?) に置換
    pattern = re.sub(r"\$\{(\w+)\}", r"(?P<\1>.+?)", fmt)
    # 完全一致を要求
    pattern = "^" + pattern + "$"
    m = re.match(pattern, s)
    return m.groupdict() if m else None


def load_policy_file(policy_name: str) -> Dict[str, Any]:
    """Load policy YAML file from manifests directory"""
    file_path = Path(__file__).parent / "manifests" / f"{policy_name}.yaml"
    with open(file_path, "r") as f:
        return yaml.safe_load(f)


def load_policies(namespace: str = None) -> List[Any]:
    """Load policy from Kubernetes cluster"""
    try:
        api = client.CustomObjectsApi()

        if namespace:
            # Get scores from specific namespace
            result = api.list_namespaced_custom_object(
                group="policy.open-cluster-management.io",
                version="v1",
                namespace=namespace,
                plural="policies",
            )
        else:
            # Get all scores from all namespaces
            result = api.list_cluster_custom_object(
                group="policy.open-cluster-management.io",
                version="v1",
                plural="policies",
            )

        return result.get("items", [])
    except Exception as e:
        print(f"Error loading policy: {e}")
        return []


def load_workload_file(workload_name: str) -> Dict[str, Any]:
    """Load workload YAML file from manifests directory"""
    file_path = Path(__file__).parent / "manifests" / f"{workload_name}.yaml"
    with open(file_path, "r") as f:
        return yaml.safe_load(f)


def load_workloads(namespace: str = None) -> List[Any]:
    """Load workloads from Kubernetes cluster"""
    try:
        api = client.CustomObjectsApi()

        if namespace:
            # Get scores from specific namespace
            result = api.list_namespaced_custom_object(
                group="work.open-cluster-management.io",
                version="v1alpha1",
                namespace=namespace,
                plural="manifestworkreplicasets",
            )
        else:
            # Get all scores from all namespaces
            result = api.list_cluster_custom_object(
                group="work.open-cluster-management.io",
                version="v1alpha1",
                plural="manifestworkreplicasets",
            )

        return result.get("items", [])
    except Exception as e:
        print(f"Error loading workload: {e}")
        return []


def load_addonplacementscore_file(cluster_name: str, score_name: str) -> Dict[str, Any]:
    """Load AddonPlacementScore YAML file from manifests directory"""
    file_path = (
        Path(__file__).parent / "manifests" / f"{score_name}-{cluster_name}.yaml"
    )
    with open(file_path, "r") as f:
        return yaml.safe_load(f)


def load_addonplacementscores(namespace: str = None) -> List[Any]:
    """Load addon placement scores from Kubernetes cluster"""
    try:
        api = client.CustomObjectsApi()

        scorers = api.list_cluster_custom_object(
            group="dynamic-scoring.open-cluster-management.io",
            version="v1",
            plural="dynamicscorers",
        )

        descriptions = {}
        for scorer in scorers.get("items", []):
            name = scorer["metadata"]["name"]
            if scorer["spec"]["configSyncMode"] != "Full":
                score_name = (
                    scorer["spec"]
                    .get("scoring", {})
                    .get("params", {})
                    .get("name", name)
                )
            else:
                score_name = (
                    scorer["status"]
                    .get("lastSyncedConfig", {})
                    .get("scoring", {})
                    .get("params", {})
                    .get("name", name)
                )
            score_cr_name = score_name.replace("_", "-")

            description = scorer["spec"].get("description", "")
            score_dimension_format = scorer["spec"].get("scoreDimensionFormat", "")
            descriptions[score_cr_name] = {
                "description": description,
                "scoreDimensionFormat": score_dimension_format,
            }

        print(f"Loaded descriptions: {descriptions}")

        if namespace:
            # Get scores from specific namespace
            result = api.list_namespaced_custom_object(
                group="cluster.open-cluster-management.io",
                version="v1alpha1",
                namespace=namespace,
                plural="addonplacementscores",
            )
        else:
            # Get all scores from all namespaces
            result = api.list_cluster_custom_object(
                group="cluster.open-cluster-management.io",
                version="v1alpha1",
                plural="addonplacementscores",
            )
        scores = result.get("items", [])

        # Add descriptions to scores
        for score in scores:
            score_cr_name = score["metadata"]["name"].replace("_", "-")
            score["metadata"] = {
                "name": score["metadata"]["name"],
                "namespace": score["metadata"]["namespace"],
                "labels": score["metadata"].get("labels", {}),
                "annotations": score["metadata"].get("annotations", {}),
            }
            if score_cr_name in descriptions:
                score["metadata"]["annotations"][
                    "dynamic-scoring.open-cluster-management.io/description"
                ] = descriptions[score_cr_name]["description"]
                score["metadata"]["annotations"][
                    "dynamic-scoring.open-cluster-management.io/score-dimension-format"
                ] = descriptions[score_cr_name]["scoreDimensionFormat"]

        return scores
    except Exception as e:
        print(f"Error loading workload: {e}")
        return []


def render_template(template_name: str, **kwargs) -> str:
    """Render Jinja2 template from templates directory"""
    template_path = Path(__file__).parent / "templates" / template_name
    with open(template_path, "r") as f:
        template = jinja2.Template(f.read())
    return template.render(**kwargs)


def perform_optimization(params: OptimizationParams) -> OptimizationResponse:
    """Perform optimization based on parameters"""

    namespace = params.namespace
    apps = set()
    clusters = set()
    devices = set()
    resource_capacity = dict()
    resource_request = dict()
    preference_scores = dict()
    policy_map = dict()
    workload_placement_map = dict()

    target_policies = load_policies(namespace)
    target_policy_map = {p["metadata"]["name"]: p for p in target_policies}
    target_workloads = load_workloads(namespace)
    target_workload_map = {w["metadata"]["name"]: w for w in target_workloads}
    addonplacementscores = load_addonplacementscores()
    addonplacementscore_map = {
        (a["metadata"]["namespace"], a["metadata"]["name"]): a
        for a in addonplacementscores
    }

    print(
        f"Loaded {len(target_policies)} policies, {len(target_workloads)} workloads, {len(addonplacementscores)} addon placement scores"
    )
    print(addonplacementscore_map)

    # Process clusters
    for cluster in params.clusters:
        cluster_name = cluster["name"]
        clusters.add(cluster_name)
        for availablePolicy in cluster["availablePolicies"]:
            print(f"Processing policy: {availablePolicy['name']}")
            policy_data = target_policy_map[availablePolicy["name"]]
            resource_info = {
                "kind": policy_data["metadata"]["labels"]["resource-supply-kind"],
                "amount": policy_data["metadata"]["labels"]["resource-supply-amount"],
                "device": policy_data["metadata"]["labels"]["resource-supply-device"],
            }
            devices.add(resource_info["device"])
            resource_capacity[(cluster_name, resource_info["device"])] = int(
                resource_info["amount"]
            )
            policy_map[(cluster_name, resource_info["device"])] = availablePolicy[
                "name"
            ]

    # Process target workloads
    for target_workload in params.targetWorkloads:
        print(f"Processing workload: {target_workload['name']}")
        request_data = target_workload_map[target_workload["name"]]
        request_info = {
            "app": request_data["metadata"]["labels"]["app"],
            "kind": request_data["metadata"]["labels"]["resource-request-kind"],
            "amount": request_data["metadata"]["labels"]["resource-request-amount"],
        }
        apps.add(request_info["app"])
        resource_request[request_info["app"]] = int(request_info["amount"])
        workload_placement_map[request_info["app"]] = request_data["spec"][
            "placementRefs"
        ][0]["name"]

    # Process preference scores
    addonPlacementScore = params.preference["addonPlacementScore"]
    scoreDimensionFormat = params.preference["scoreDimensionFormat"]

    for cluster_name in clusters:
        print(f"Processing cluster: {cluster_name} for score {addonPlacementScore}")
        score_data = addonplacementscore_map[(cluster_name, addonPlacementScore)]
        # score_data = load_addonplacementscore_file(cluster_name, addonPlacementScore)
        for score in score_data["status"]["scores"]:
            dimention = parse_with_format(scoreDimensionFormat, score["name"])
            if dimention:
                value = score["value"]
                preference_scores[
                    (dimention["app"], cluster_name, dimention["device"])
                ] = value

    # --- Optimization Model ---

    print("Loaded data:")
    print(f"Apps: {len(apps)}")
    print(f"Clusters: {len(clusters)}")
    print(f"Devices: {len(devices)}")

    model = pulp.LpProblem("Resource_Assignment", pulp.LpMaximize)

    # Variables
    x = pulp.LpVariable.dicts(
        "x",
        [(a, c) for a in apps for c in clusters],
        lowBound=0,
        upBound=1,
        cat=pulp.LpBinary,
    )

    y = pulp.LpVariable.dicts(
        "y",
        [(c, d) for c in clusters for d in devices],
        lowBound=0,
        upBound=1,
        cat=pulp.LpBinary,
    )

    z = pulp.LpVariable.dicts(
        "z",
        [(a, c, d) for a in apps for c in clusters for d in devices],
        lowBound=0,
        upBound=1,
        cat=pulp.LpBinary,
    )

    # Objective function
    model += (
        pulp.lpSum(
            preference_scores[(a, c, d)] * z[(a, c, d)]
            for a in apps
            for c in clusters
            for d in devices
        ),
        "Total_Performance",
    )

    # Constraints
    # 1. Each resource chooses at most one type
    for c in clusters:
        model += (
            pulp.lpSum(y[(c, d)] for d in devices) <= 1,
            f"OneType_per_resource_{c}",
        )

    # 2. Each job assigned to at most one resource
    for a in apps:
        model += (
            pulp.lpSum(x[(a, c)] for c in clusters) <= 1,
            f"OneResource_per_job_{a}",
        )

    # 3. Capacity constraints
    for c in clusters:
        for d in devices:
            model += (
                (
                    pulp.lpSum(resource_request[a] * z[(a, c, d)] for a in apps)
                    <= resource_capacity[(c, d)] * y[(c, d)]
                ),
                f"Capacity_{c}_{d}",
            )

    # 4. Link constraints
    for a in apps:
        for c in clusters:
            for d in devices:
                model += z[(a, c, d)] <= x[(a, c)], f"z_le_x_{a}_{c}_{d}"
                model += z[(a, c, d)] <= y[(c, d)], f"z_le_y_{a}_{c}_{d}"
                model += (
                    z[(a, c, d)] >= x[(a, c)] + y[(c, d)] - 1,
                    f"z_ge_x_plus_y_minus1_{a}_{c}_{d}",
                )

    # 5. Consistency: x_{a,c} = sum_d z_{a,c,d}
    for a in apps:
        for c in clusters:
            model += (
                (x[(a, c)] == pulp.lpSum(z[(a, c, d)] for d in devices)),
                f"x_equals_sum_z_{a}_{c}",
            )

    # Solve
    model.solve(pulp.PULP_CBC_CMD(msg=0))

    status = pulp.LpStatus[model.status]
    objective_value = (
        pulp.value(model.objective) if model.status == pulp.LpStatusOptimal else None
    )

    # Generate output
    exec_hash = secrets.token_hex(4)
    placement_for_policies = []
    placement_bindings = []

    for c in clusters:
        for d in devices:
            if pulp.value(y[(c, d)]) > 0.5:
                placement_for_policies.append(
                    {
                        "name": f"placement-{c}-{exec_hash}",
                        "namespace": namespace,
                        "cluster_name": c,
                    }
                )
                placement_bindings.append(
                    {
                        "name": f"binding-{c}-{exec_hash}",
                        "namespace": namespace,
                        "placement_name": f"placement-{c}-{exec_hash}",
                        "policy_name": policy_map[(c, d)],
                    }
                )

    placement_for_workloads = []
    for a in apps:
        for c in clusters:
            if pulp.value(x[(a, c)]) > 0.5:
                placement_for_workloads.append(
                    {
                        "name": workload_placement_map[a],
                        "namespace": namespace,
                        "cluster_name": c,
                    }
                )

    # Render templates
    res = []
    for p in placement_for_policies:
        res.append(render_template("placement-policy.yaml", **p))
    for p in placement_bindings:
        res.append(render_template("placementbinding.yaml", **p))
    for p in placement_for_workloads:
        res.append(render_template("placement-app.yaml", **p))

    # Create resulted Placement with

    output_yaml = "\n---\n".join(res)

    obj = yaml.safe_load_all(output_yaml)

    if status == "Optimal":
        print("Optimization successful. Creating resources...")
        api = client.CustomObjectsApi()
        for o in obj:
            try:
                if o["kind"] == "Placement":
                    api.create_namespaced_custom_object(
                        group="cluster.open-cluster-management.io",
                        version="v1beta1",
                        namespace=o["metadata"]["namespace"],
                        plural="placements",
                        body=o,
                    )
                elif o["kind"] == "PlacementBinding":
                    api.create_namespaced_custom_object(
                        group="policy.open-cluster-management.io",
                        version="v1",
                        namespace=o["metadata"]["namespace"],
                        plural="placementbindings",
                        body=o,
                    )
            except client.ApiException as e:
                print(f"Failed to create {o['kind']} {o['metadata']['name']}: {e}")
            except Exception as e:
                print(
                    f"Unexpected error creating {o['kind']} {o['metadata']['name']}: {e}"
                )

    return OptimizationResponse(
        status=status,
        objective_value=objective_value,
        output_yaml=output_yaml,
    )


app = FastAPI()

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


@app.get("/samples", response_model=GetSampleResponse)
async def get_sample(sample_name: Optional[str] = None) -> GetSampleResponse:
    """Get sample message"""

    namespace = "default"

    a = load_policies(namespace)
    b = load_workloads(namespace)
    c = load_addonplacementscores()

    print(
        f"Loaded {len(a)} policies, {len(b)} workloads, {len(c)} addon placement scores"
    )

    return GetSampleResponse(
        message=f"Sample response message. Sample name: {sample_name}"
    )


@app.get("/addonplacementscores", response_model=AddonPlacementScoreResponse)
async def get_addonplacementscores(
    namespace: Optional[str] = None,
) -> AddonPlacementScoreResponse:
    """Get AddonPlacementScores from the cluster"""

    try:
        scores = load_addonplacementscores()
        return AddonPlacementScoreResponse(items=scores)

    except client.ApiException as e:
        raise HTTPException(
            status_code=e.status,
            detail=f"Failed to get AddonPlacementScores: {e.reason}",
        )
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Unexpected error: {str(e)}")


@app.post("/optimize", response_model=OptimizationResponse)
async def optimize(params: OptimizationParams) -> OptimizationResponse:
    """Perform resource optimization based on parameters

    This endpoint accepts optimization parameters and returns the optimized
    placement configuration in YAML format.
    """
    try:
        return perform_optimization(params)
    except FileNotFoundError as e:
        raise HTTPException(
            status_code=404,
            detail=f"Required file not found: {str(e)}",
        )
    except Exception as e:
        raise HTTPException(
            status_code=500,
            detail=f"Optimization failed: {str(e)}",
        )


mcp = FastApiMCP(app)
mcp.mount_http()

if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=8338)
