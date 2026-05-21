import pulp
import yaml
import secrets
import jinja2

import re


def parse_with_format(fmt: str, s: str):
    # Replace ${name} with (?P<name>.+?)
    pattern = re.sub(r"\$\{(\w+)\}", r"(?P<\1>.+?)", fmt)

    # Require full match
    pattern = "^" + pattern + "$"

    m = re.match(pattern, s)
    return m.groupdict() if m else None


# --- Data Definitions -------------------------------------------------

with open("params.yaml", "r") as f:
    params = yaml.safe_load(f)

namespace = params["namespace"]
apps = set()
clusters = set()
devices = set()
resource_capacity = dict()
resource_request = dict()
preference_scores = dict()
policy_map = dict()
workload_placement_map = dict()

for cluster in params["clusters"]:
    print("Cluster:", cluster["name"])
    clusters.add(cluster["name"])
    for availablePolicy in cluster["availablePolicies"]:
        print("  Available Policy:", availablePolicy)
        with open(f"manifests/{availablePolicy['name']}.yaml", "r") as pf:
            policy_data = yaml.safe_load(pf)
            resource_info = {
                "kind": policy_data["metadata"]["labels"]["resource-supply-kind"],
                "amount": policy_data["metadata"]["labels"]["resource-supply-amount"],
                "device": policy_data["metadata"]["labels"]["resource-supply-device"],
            }
            print("    Resource Info:", resource_info)
            devices.add(resource_info["device"])
            resource_capacity[(cluster["name"], resource_info["device"])] = int(
                resource_info["amount"]
            )
            policy_map[(cluster["name"], resource_info["device"])] = availablePolicy[
                "name"
            ]

for target_workload in params["targetWorkloads"]:
    print("Target:", target_workload["name"])
    with open(f"manifests/{target_workload['name']}.yaml", "r") as rp:
        request_data = yaml.safe_load(rp)
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
        print("    Request Info:", request_info)

addonPlacementScore = params["preference"]["addonPlacementScore"]
scoreDimensionFormat = params["preference"]["scoreDimensionFormat"]

cluster_names = [c["name"] for c in params["clusters"]]
for cluster_name in clusters:
    with open(f"manifests/{addonPlacementScore}-{cluster_name}.yaml", "r") as sf:
        score_data = yaml.safe_load(sf)
        for score in score_data["status"]["scores"]:
            dimention = parse_with_format(scoreDimensionFormat, score["name"])
            value = score["value"]
            print(
                f"Cluster: {cluster_name}, Score Name: {score['name']}, Dimension: {dimention}, Value: {value}"
            )
            preference_scores[(dimention["app"], cluster_name, dimention["device"])] = (
                value
            )

print("Apps:", apps)
print("Clusters:", clusters)
print("Devices:", devices)
print("Preference Scores:", preference_scores)
print("Resource Capacity:", resource_capacity)

# --- Model Definition -------------------------------------------------

model = pulp.LpProblem("Resource_Assignment", pulp.LpMaximize)

# Variable x_{a,c}: whether job a is assigned to resource c
x = pulp.LpVariable.dicts(
    "x",
    [(a, c) for a in apps for c in clusters],
    lowBound=0,
    upBound=1,
    cat=pulp.LpBinary,
)

# Variable y_{c,d}: whether resource c selects type d
y = pulp.LpVariable.dicts(
    "y",
    [(c, d) for c in clusters for d in devices],
    lowBound=0,
    upBound=1,
    cat=pulp.LpBinary,
)

# Variable z_{a,c,d}: whether job a is assigned to resource c running on type d
z = pulp.LpVariable.dicts(
    "z",
    [(a, c, d) for a in apps for c in clusters for d in devices],
    lowBound=0,
    upBound=1,
    cat=pulp.LpBinary,
)

# --- Objective Function ---------------------------------------------------

model += (
    pulp.lpSum(
        preference_scores[(a, c, d)] * z[(a, c, d)]
        for a in apps
        for c in clusters
        for d in devices
    ),
    "Total_Performance",
)

# --- Constraints -------------------------------------------------------

# 1. Each resource selects at most one type
for c in clusters:
    model += pulp.lpSum(y[(c, d)] for d in devices) <= 1, f"OneType_per_resource_{c}"
# 2. Each job is assigned to at most one resource
for a in apps:
    model += pulp.lpSum(x[(a, c)] for c in clusters) <= 1, f"OneResource_per_job_{a}"

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

# 4. Linking constraints: z <= x, z <= y, z >= x + y - 1
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

# --- Solve -------------------------------------------------------

print("Solving the model...")

model.solve(pulp.PULP_CBC_CMD(msg=0))

print("Status:", pulp.LpStatus[model.status])
print("Objective (Total Performance):", pulp.value(model.objective))

# Display results
print("\nCluster Device Type Assignments:")

exec_hash = secrets.token_hex(4)
placement_for_policies = []
placement_bindings = []
for c in clusters:
    for d in devices:
        if pulp.value(y[(c, d)]) > 0.5:
            print(f"  {c} -> Device Type {d}")
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

print("\nWorkload Assignments:")
placement_for_workloads = []
for a in apps:
    for c in clusters:
        if pulp.value(x[(a, c)]) > 0.5:
            # Check which device type the job is running on
            assigned_types = [d for d in devices if pulp.value(z[(a, c, d)]) > 0.5]
            t_str = ",".join(assigned_types) if assigned_types else "None"
            print(f"  {a} -> {c} (device type={t_str})")
            placement_for_workloads.append(
                {
                    "name": workload_placement_map[a],
                    "namespace": namespace,
                    "cluster_name": c,
                }
            )

# --- Output Results -----------------------------------------------------
print("\nOutput written to output.yaml")

res = []
for p in placement_for_policies:
    res.append(
        jinja2.Template(open("templates/placement-policy.yaml").read()).render(**p)
    )
for p in placement_bindings:
    res.append(
        jinja2.Template(open("templates/placementbinding.yaml").read()).render(**p)
    )
for p in placement_for_workloads:
    res.append(jinja2.Template(open("templates/placement-app.yaml").read()).render(**p))
with open("output.yaml", "w") as f:
    f.write("\n---\n".join(res))
