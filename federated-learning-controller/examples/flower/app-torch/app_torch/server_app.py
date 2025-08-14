"""Torch: A Flower / PyTorch Federated Learning Server."""

import argparse
import os
import torch
import json
import numpy as np
from datetime import datetime
from collections import OrderedDict
from typing import List, Tuple

import flwr as fl
from flwr.common import Metrics, ndarrays_to_parameters
from flwr.server import ServerConfig
from flwr.server.strategy import FedAvg

from app_torch.task import Net, get_weights
from app_torch.utils import get_latest_model_file, load_model, save_model


# ------------------------------
# Argument Parsing
# ------------------------------

def parse_arguments():
    """Parses command-line arguments for the FL server."""
    parser = argparse.ArgumentParser(description="Start Federated Learning server.")
    parser.add_argument("--server-address", type=str, default="0.0.0.0:8080", help="Server address.")
    parser.add_argument("--num-rounds", type=int, default=10, help="Number of training rounds.")
    parser.add_argument("--min-available-clients", type=int, default=2, help="Minimum number of clients required for training.")
    parser.add_argument("--model-dir", type=str, default="/data/models", help="Path to save models.", required=False)
    parser.add_argument("--init-model", type=str, default="", help="Path to initial model.", required=False)
    
    return parser.parse_args()


# ------------------------------
# Federated Learning Server
# ------------------------------

def client_weighted_average(metrics: List[Tuple[int, Metrics]]) -> Metrics:
    """Compute weighted average of client accuracy."""
    accuracies = [num_examples * metric["accuracy"] for num_examples, metric in metrics]
    examples = [num_examples for num_examples, _ in metrics]
    return {"accuracy": sum(accuracies) / sum(examples)}

def fit_config(server_round: int):
    config = {
        "server_round": server_round,
    }
    return config

def start_server(args):
    """Starts the federated learning server with model aggregation and checkpointing."""
    model = Net()

    # Ensure model directory exists
    os.makedirs(args.model_dir, exist_ok=True)

    # Load or initialize model
    last_model_file = get_latest_model_file(args.model_dir)
    if last_model_file:
        print(f"Loading model from {last_model_file}")
        model = load_model(model, last_model_file)
    elif args.init_model:
        print(f"Loading initial model from {args.init_model}")
        model = load_model(model, os.path.join(args.model_dir, args.init_model))
    else:
        save_model(model, os.path.join(args.model_dir, "model_init.pth"))

    # Convert model weights to Flower format
    initial_parameters = ndarrays_to_parameters(get_weights(model))

    # Custom aggregation strategy to save the latest aggregated model
    class SaveLatestModelStrategy(FedAvg):
        def aggregate_fit(self, rnd, results, failures):
            """Aggregate model weights using weighted average and store checkpoint."""
            aggregated_parameters, aggregated_metrics = super().aggregate_fit(rnd, results, failures)

            if aggregated_parameters is not None:
                print(f"Saving aggregated model after round {rnd}...")
                net = Net()
                aggregated_ndarrays = fl.common.parameters_to_ndarrays(aggregated_parameters)

                # Convert to PyTorch state_dict
                params_dict = zip(net.state_dict().keys(), aggregated_ndarrays)
                state_dict = OrderedDict({k: torch.tensor(v) for k, v in params_dict})
                net.load_state_dict(state_dict, strict=True)

                # Save model checkpoint
                model_time = datetime.now().strftime('%Y-%m-%d-%H-%M-%S')
                model_file = os.path.join(args.model_dir, f"model_round_{rnd}_{model_time}.pth")
                print(f"Model saved at: {model_file}")
                torch.save(net.state_dict(), model_file)

            return aggregated_parameters, aggregated_metrics

        def aggregate_evaluate(self, server_round, results, failures):
            aggregated_loss, aggregated_metrics = super().aggregate_evaluate(server_round, results, failures)
            metrics = {
                "round": server_round,
                "loss": aggregated_loss,
                "accuracy": aggregated_metrics["accuracy"],
            }
            try:
                os.makedirs('/metrics', exist_ok=True)
                with open('/metrics/metric.json', 'w', encoding='utf-8') as f:
                    json.dump(metrics, f, ensure_ascii=False)
            except Exception as e:
                print("write json file error: ", e)
            return aggregated_loss, aggregated_metrics

    # Start the FL server
    fl.server.start_server(
        server_address=args.server_address,
        config=ServerConfig(num_rounds=args.num_rounds),
        strategy=SaveLatestModelStrategy(
            fraction_fit=1.0,
            fraction_evaluate=1.0,
            min_available_clients=args.min_available_clients,
            initial_parameters=initial_parameters,
            evaluate_metrics_aggregation_fn=client_weighted_average,
            inplace=True,
            on_fit_config_fn=fit_config,
        ),
    )


if __name__ == "__main__":
    args = parse_arguments()
    print(vars(args))  # Print parsed arguments for debugging
    start_server(args)
