#!/bin/sh

# docker run --rm image server --num-rounds 20
# docker run --rm image client --data-config "data-cluster1" --server-address 172.17.0.2:8080
# pip install -e .

# Set writable HOME and ensure /app/.venv has proper permissions
export HOME=/app
export PIP_NO_CACHE_DIR=1
export VENV_PATH=/app/.venv

# Ensure /app/.venv is writable by the current user
mkdir -p $VENV_PATH
chmod -R 777 $VENV_PATH

# Install dependencies globally but within /app/.venv
pip install --no-cache-dir --prefix=$VENV_PATH -e .

# Update PYTHONPATH to include the new virtual environment
export PYTHONPATH=$VENV_PATH/lib/python3.11/site-packages:/app:$PYTHONPATH

echo "Installed the package successfully!"

# Map input commands to the appropriate Python script
if [ "$1" = "server" ]; then
  shift
  exec python app_torch/server_app.py "$@"
elif [ "$1" = "client" ]; then
  shift
  exec python app_torch/client_app.py "$@"
else
  echo "Error: Unsupported command '$1'. Use 'server' or 'client'."
  exit 1
fi
