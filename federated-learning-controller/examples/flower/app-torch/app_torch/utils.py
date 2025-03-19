import os
from datetime import datetime
import torch


def load_model(model, model_path):
    if os.path.exists(model_path):
        model.load_state_dict(torch.load(model_path, weights_only=True))
        return model
    print("No model found. Initializing a new model.")
    return model
  
def save_model(model, model_path):
    print(f"Saving model to {model_path}...")
    torch.save(model.state_dict(), model_path)

import os
from datetime import datetime

def get_latest_model_file(model_dir="/data/model", suffix=".pth"):
    if not os.path.exists(model_dir):
        return None

    model_files = []
    for filename in os.listdir(model_dir):
        filepath = os.path.join(model_dir, filename)
        if os.path.isfile(filepath):
            # Check for "init.*" or timestamp patterns like "YYYY-MM-DD-HH-MM-SS.*"
            if filename.endswith(suffix):
                try:
                    if filename.startswith("init."):
                        # Assign "init.*" a high priority timestamp
                        timestamp = datetime.min
                    else:
                        timestamp = datetime.strptime(filename[:19], "%Y-%m-%d-%H-%M-%S")
                    model_files.append((timestamp, filepath))
                except ValueError:
                    # Skip files that don't match the expected timestamp format
                    pass

    if not model_files:
        return None

    # Sort by timestamp and get the latest
    latest_model = max(model_files, key=lambda x: x[0])[1]
    return latest_model

# file = get_latest_model_file("/home/myan/workspace/federated-learning")
# print(file)