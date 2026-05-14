"""app-torch: A Flower / PyTorch ClientApp for MNIST."""

import hashlib

import torch
from flwr.app import ArrayRecord, Context, Message, MetricRecord, RecordDict
from flwr.clientapp import ClientApp

from app_torch.task import Net, load_data
from app_torch.task import test as test_fn
from app_torch.task import train as train_fn

app = ClientApp()


def get_partition_id(context: Context) -> int:
    """Resolve partition-id from node config.

    Supports:
    1. Explicit "partition-id" (manual configuration).
    2. sha256("cluster-name") (auto-injected by the OCM flower-addon
       SuperNode). Python's built-in hash() is salted per process, so it
       would assign the same cluster to a different partition after a pod
       restart — sha256 keeps the mapping stable.
    """
    node_config = context.node_config
    num_partitions = int(node_config["num-partitions"])

    if "partition-id" in node_config:
        return int(node_config["partition-id"]) % num_partitions

    if "cluster-name" in node_config:
        digest = hashlib.sha256(
            str(node_config["cluster-name"]).encode("utf-8")
        ).digest()
        return int.from_bytes(digest[:8], "big") % num_partitions

    raise ValueError("Either 'partition-id' or 'cluster-name' must be in node_config")


@app.train()
def train(msg: Message, context: Context):
    """Train the model on the local partition."""
    model = Net()
    model.load_state_dict(msg.content["arrays"].to_torch_state_dict())
    device = torch.device("cuda:0" if torch.cuda.is_available() else "cpu")
    model.to(device)

    partition_id = get_partition_id(context)
    num_partitions = int(context.node_config["num-partitions"])
    batch_size = context.run_config["batch-size"]
    trainloader, _ = load_data(partition_id, num_partitions, batch_size)

    train_loss = train_fn(
        model,
        trainloader,
        context.run_config["local-epochs"],
        msg.content["config"]["lr"],
        device,
    )

    model_record = ArrayRecord(model.state_dict())
    metric_record = MetricRecord(
        {"train_loss": train_loss, "num-examples": len(trainloader.dataset)}
    )
    content = RecordDict({"arrays": model_record, "metrics": metric_record})
    return Message(content=content, reply_to=msg)


@app.evaluate()
def evaluate(msg: Message, context: Context):
    """Evaluate the aggregated model on the local partition."""
    model = Net()
    model.load_state_dict(msg.content["arrays"].to_torch_state_dict())
    device = torch.device("cuda:0" if torch.cuda.is_available() else "cpu")
    model.to(device)

    partition_id = get_partition_id(context)
    num_partitions = int(context.node_config["num-partitions"])
    batch_size = context.run_config["batch-size"]
    _, valloader = load_data(partition_id, num_partitions, batch_size)

    eval_loss, eval_acc = test_fn(model, valloader, device)

    metric_record = MetricRecord(
        {
            "eval_loss": eval_loss,
            "eval_acc": eval_acc,
            "num-examples": len(valloader.dataset),
        }
    )
    content = RecordDict({"metrics": metric_record})
    return Message(content=content, reply_to=msg)
