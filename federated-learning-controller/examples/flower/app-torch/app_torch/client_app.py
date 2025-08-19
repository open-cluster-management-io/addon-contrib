"""torch: A Flower / PyTorch app."""

import torch
import torch.optim as optim
from torch.optim.lr_scheduler import StepLR

from flwr.client import ClientApp, NumPyClient
from flwr.common import Context
from app_torch.task import Net, get_weights, load_data, set_weights, test, train
from app_torch.utils import write_metrics


# Define Flower Client and client_fn
class FlowerClient(NumPyClient):
    def __init__(self, net, trainloader, valloader, local_epochs):
        self.net = net
        self.trainloader = trainloader
        self.valloader = valloader
        
        self.local_epochs = local_epochs
        self.device = torch.device("cuda:0" if torch.cuda.is_available() else "cpu")
        self.optimizer = optim.Adadelta(net.parameters(), lr=1.0)
        self.scheduler = StepLR(self.optimizer, step_size=1, gamma=0.7)

    def fit(self, parameters, config):
        set_weights(self.net, parameters)
      
        optimizer = optim.Adadelta(self.net.parameters(), lr=1.0)
        scheduler = StepLR(optimizer, step_size=1, gamma=0.7)

        epoch_loss = 0.0
        for epoch in range(1, self.local_epochs + 1):
            # test_loss = test(model, device, test_loader)
            # test_losses.append(test_loss)
            loss = train(self.net, self.device, self.trainloader, optimizer, epoch)
            metrics = {
                "round": config["server_round"],
                "epoch": epoch,
                "train_loss": loss,
            }
            write_metrics(metrics)
            epoch_loss += loss
            scheduler.step()
            
        return (
            get_weights(self.net),
            len(self.trainloader.dataset),
            {"train_loss": epoch_loss / self.local_epochs},
        )

    def evaluate(self, parameters, config):
        set_weights(self.net, parameters)
        
        loss, accuracy = test(self.net, self.device, self.valloader)
        print(f"Evaluation Loss: {loss}, Accuracy: {accuracy}")
        metrics = {
            "round": config["server_round"],
            "evaluation_loss": loss,
            "accuracy": accuracy,
        }
        write_metrics(metrics)
        return loss, len(self.valloader.dataset), {"accuracy": accuracy}

def client_fn(context: Context):
    # Load model and data
    net = Net()
    cluster_config = context.node_config["cluster-data-config"]
    local_epochs = context.run_config["local-epochs"]

    trainloader, valloader = load_data(cluster_config)
    print(f"Cluster {cluster_config} -> epochs: {local_epochs}")
    # Return Client instance
    return FlowerClient(net, trainloader, valloader, local_epochs).to_client()


# add run client 
import flwr as fl 
import argparse
import time

MAX_RETRIES = 60
RETRY_DELAY = 10  # seconds
def run_client():
    """Parses arguments and starts the federated learning client with retry logic."""
    parser = argparse.ArgumentParser(description="Flower Federated Learning Client")
    parser.add_argument("--data-config", default="cluster1", type=str, help="Cluster-specific data configuration.")
    parser.add_argument("--epochs", type=int, default=1, help="Number of training epochs.")
    parser.add_argument("--server-address", type=str, default="127.0.0.1:8080", help="Federated Learning server address.")
    args = parser.parse_args()

    print(f"Using dataset: {args.data_config}")
    print(f"Training for {args.epochs} epochs")
    print(f"Connecting to server at: {args.server_address}")

    for attempt in range(MAX_RETRIES):
        try:
            fl.client.start_client(
                server_address=args.server_address,
                client=client_fn(
                    context=Context(
                        run_id="",
                        node_id="",
                        state=None,
                        node_config={"cluster-data-config": args.data_config},
                        run_config={"local-epochs": args.epochs},
                    )
                ),
            )
            print("Client started successfully.")
            break
        except Exception as e:
            print(f"Attempt {attempt + 1} failed: {e}")
            if attempt < MAX_RETRIES - 1:
                print(f"Retrying in {RETRY_DELAY} seconds...")
                time.sleep(RETRY_DELAY)
            else:
                print("Max retries reached. Exiting.")
                raise e

if __name__ == "__main__":
    run_client()