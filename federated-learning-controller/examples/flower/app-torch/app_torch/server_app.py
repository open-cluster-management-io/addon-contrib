"""app-torch: A Flower / PyTorch ServerApp for MNIST."""

import os

import torch
from flwr.app import ArrayRecord, ConfigRecord, Context, MetricRecord
from flwr.serverapp import Grid, ServerApp
from flwr.serverapp.strategy import FedAvg

from app_torch.task import Net, load_centralized_dataset, test

app = ServerApp()


@app.main()
def main(grid: Grid, context: Context) -> None:
    """Entry point for the ServerApp."""
    fraction_evaluate: float = context.run_config["fraction-evaluate"]
    num_rounds: int = context.run_config["num-server-rounds"]
    lr: float = context.run_config["learning-rate"]

    global_model = Net()
    arrays = ArrayRecord(global_model.state_dict())

    strategy = FedAvg(fraction_evaluate=fraction_evaluate)

    result = strategy.start(
        grid=grid,
        initial_arrays=arrays,
        train_config=ConfigRecord({"lr": lr}),
        num_rounds=num_rounds,
        evaluate_fn=global_evaluate,
    )

    # The ServerApp pod has no PVC by default, so /app is ephemeral. Set
    # FLOWER_MODEL_OUTPUT_PATH to a path backed by a volume mount to keep
    # the trained model after the pod exits.
    output_path = os.environ.get("FLOWER_MODEL_OUTPUT_PATH")
    if output_path:
        print(f"\nSaving final model to {output_path}...")
        torch.save(result.arrays.to_torch_state_dict(), output_path)
    else:
        print("\nFLOWER_MODEL_OUTPUT_PATH is not set, skipping model save.")


def global_evaluate(server_round: int, arrays: ArrayRecord) -> MetricRecord:
    """Evaluate the aggregated model on a centralized MNIST test set."""
    model = Net()
    model.load_state_dict(arrays.to_torch_state_dict())
    device = torch.device("cuda:0" if torch.cuda.is_available() else "cpu")
    model.to(device)

    test_loader = load_centralized_dataset()
    test_loss, test_acc = test(model, test_loader, device)
    return MetricRecord({"accuracy": test_acc, "loss": test_loss})
