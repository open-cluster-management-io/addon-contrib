"""cifar10: A Flower / PyTorch app."""

import torch
from flwr.app import ArrayRecord, Context, Message, MetricRecord, RecordDict
from flwr.clientapp import ClientApp

from cifar10.task import Net, load_data
from cifar10.task import test as test_fn
from cifar10.task import train as train_fn

# Flower ClientApp
app = ClientApp()


def get_partition_id(context: Context) -> int:
    """Get partition ID from node config.

    Supports two modes:
    1. Direct partition-id (for manual configuration)
    2. Derived from cluster-name using hash (for OCM auto-install)
    """
    node_config = context.node_config
    num_partitions = int(node_config["num-partitions"])

    # Check if partition-id is directly specified
    if "partition-id" in node_config:
        return int(node_config["partition-id"])

    # Derive partition-id from cluster-name
    if "cluster-name" in node_config:
        cluster_name = node_config["cluster-name"]
        # Use hash to get consistent partition ID for each cluster
        partition_id = hash(cluster_name) % num_partitions
        return partition_id

    raise ValueError("Either 'partition-id' or 'cluster-name' must be in node_config")


@app.train()
def train(msg: Message, context: Context):
    """Train the model on local data."""

    # Load the model and initialize it with the received weights
    model = Net()
    model.load_state_dict(msg.content["arrays"].to_torch_state_dict())
    device = torch.device("cuda:0" if torch.cuda.is_available() else "cpu")
    model.to(device)

    # Load the data
    partition_id = get_partition_id(context)
    num_partitions = int(context.node_config["num-partitions"])
    batch_size = context.run_config["batch-size"]
    trainloader, _ = load_data(partition_id, num_partitions, batch_size)

    # Call the training function
    train_loss = train_fn(
        model,
        trainloader,
        context.run_config["local-epochs"],
        msg.content["config"]["lr"],
        device,
    )

    # Construct and return reply Message
    model_record = ArrayRecord(model.state_dict())
    metrics = {
        "train_loss": train_loss,
        "num-examples": len(trainloader.dataset),
    }
    metric_record = MetricRecord(metrics)
    content = RecordDict({"arrays": model_record, "metrics": metric_record})
    return Message(content=content, reply_to=msg)


@app.evaluate()
def evaluate(msg: Message, context: Context):
    """Evaluate the model on local data."""

    # Load the model and initialize it with the received weights
    model = Net()
    model.load_state_dict(msg.content["arrays"].to_torch_state_dict())
    device = torch.device("cuda:0" if torch.cuda.is_available() else "cpu")
    model.to(device)

    # Load the data
    partition_id = get_partition_id(context)
    num_partitions = int(context.node_config["num-partitions"])
    batch_size = context.run_config["batch-size"]
    _, valloader = load_data(partition_id, num_partitions, batch_size)

    # Call the evaluation function
    eval_loss, eval_acc = test_fn(
        model,
        valloader,
        device,
    )

    # Construct and return reply Message
    metrics = {
        "eval_loss": eval_loss,
        "eval_acc": eval_acc,
        "num-examples": len(valloader.dataset),
    }
    metric_record = MetricRecord(metrics)
    content = RecordDict({"metrics": metric_record})
    return Message(content=content, reply_to=msg)
