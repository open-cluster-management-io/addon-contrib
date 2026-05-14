"""app-torch: A Flower / PyTorch MNIST app."""

import torch
import torch.nn as nn
import torch.nn.functional as F
from flwr_datasets import FederatedDataset
from flwr_datasets.partitioner import IidPartitioner
from torch.utils.data import DataLoader
from torchvision.transforms import Compose, Normalize, ToTensor


class Net(nn.Module):
    """Simple CNN for MNIST."""

    def __init__(self):
        super(Net, self).__init__()
        self.conv1 = nn.Conv2d(1, 32, 3, 1)
        self.conv2 = nn.Conv2d(32, 64, 3, 1)
        self.dropout1 = nn.Dropout(0.25)
        self.dropout2 = nn.Dropout(0.5)
        self.fc1 = nn.Linear(9216, 128)
        self.fc2 = nn.Linear(128, 10)

    def forward(self, x):
        x = self.conv1(x)
        x = F.relu(x)
        x = self.conv2(x)
        x = F.relu(x)
        x = F.max_pool2d(x, 2)
        x = self.dropout1(x)
        x = torch.flatten(x, 1)
        x = self.fc1(x)
        x = F.relu(x)
        x = self.dropout2(x)
        x = self.fc2(x)
        return F.log_softmax(x, dim=1)


fds = None  # Cache FederatedDataset

mnist_transforms = Compose([ToTensor(), Normalize((0.1307,), (0.3081,))])


def apply_transforms(batch):
    """Apply transforms to the partition from FederatedDataset."""
    batch["image"] = [mnist_transforms(img) for img in batch["image"]]
    return batch


def load_data(partition_id: int, num_partitions: int, batch_size: int):
    """Load a partition of MNIST data for one client."""
    global fds
    if fds is None:
        partitioner = IidPartitioner(num_partitions=num_partitions)
        fds = FederatedDataset(
            dataset="ylecun/mnist",
            partitioners={"train": partitioner},
        )
    partition = fds.load_partition(partition_id)
    partition_train_test = partition.train_test_split(test_size=0.2, seed=42)
    partition_train_test = partition_train_test.with_transform(apply_transforms)
    trainloader = DataLoader(
        partition_train_test["train"], batch_size=batch_size, shuffle=True
    )
    testloader = DataLoader(partition_train_test["test"], batch_size=batch_size)
    return trainloader, testloader


_centralized_loader = None


def load_centralized_dataset():
    """Load full MNIST test set for hub-side evaluation (cached after first call)."""
    global _centralized_loader
    if _centralized_loader is not None:
        return _centralized_loader

    from datasets import load_dataset

    test_dataset = load_dataset("ylecun/mnist", split="test")
    dataset = test_dataset.with_transform(apply_transforms)
    _centralized_loader = DataLoader(dataset, batch_size=128)
    return _centralized_loader


def train(net, trainloader, epochs, lr, device):
    """Train the model on the training set."""
    net.to(device)
    optimizer = torch.optim.SGD(net.parameters(), lr=lr, momentum=0.9)
    net.train()
    running_loss = 0.0
    for _ in range(epochs):
        for batch in trainloader:
            images = batch["image"].to(device)
            labels = batch["label"].to(device)
            optimizer.zero_grad()
            loss = F.nll_loss(net(images), labels)
            loss.backward()
            optimizer.step()
            running_loss += loss.item()
    return running_loss / (epochs * len(trainloader))


def test(net, testloader, device):
    """Validate the model on the test set."""
    net.to(device)
    net.eval()
    correct, loss = 0, 0.0
    with torch.no_grad():
        for batch in testloader:
            images = batch["image"].to(device)
            labels = batch["label"].to(device)
            outputs = net(images)
            loss += F.nll_loss(outputs, labels, reduction="sum").item()
            correct += (outputs.argmax(dim=1) == labels).sum().item()
    accuracy = correct / len(testloader.dataset)
    loss = loss / len(testloader.dataset)
    return loss, accuracy
