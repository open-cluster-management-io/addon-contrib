FROM flwr/superexec:1.26.1-py3.13-ubuntu24.04

WORKDIR /app
COPY pyproject.toml .
COPY app_torch/ ./app_torch/

# Bake all dependencies into the image (flwr already in base).
RUN pip install --no-cache-dir \
    "flwr-datasets[vision]>=0.6.0" \
    "torch==2.10.0" \
    "torchvision==0.25.0" \
    -e .

ENTRYPOINT ["flower-superexec"]
