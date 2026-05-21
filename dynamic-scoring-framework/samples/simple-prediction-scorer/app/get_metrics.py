import requests
import time
from datetime import datetime, timedelta
import pprint
import json

PROM_URL = "http://localhost:9090/api/v1/query_range"

QUERY = 'sum by (node, namespace, pod) (rate(container_cpu_usage_seconds_total{container!="", pod!=""}[1m]))'

# 直近1時間を30秒間隔で取得例
end_time = datetime.now()
start_time = end_time - timedelta(minutes=60)
step = 30  # 秒

params = {
    "query": QUERY,
    "start": start_time.timestamp(),
    "end": end_time.timestamp(),
    "step": step,
}

response = requests.get(PROM_URL, params=params)
data = response.json()

if data["status"] == "success":
    sample_data = data["data"]["result"]
    with open("sample_cpu_load.json", "w") as f:
        json.dump({"data": sample_data}, f, indent=2)
    for result in data["data"]["result"]:
        metric = result["metric"]
        pod_name = metric.get("pod", "unknown")
        if not pod_name.startswith("cpu-load-generator"):
            continue

        print(
            f"node={metric.get('node')}, namespace={metric.get('namespace')}, pod={metric.get('pod')}"
        )

        for value in result["values"]:
            ts = datetime.fromtimestamp(float(value[0]))
            cpu = float(value[1])
            print(f"  {ts} -> {cpu:.5f}")
else:
    print("Error:", data)
