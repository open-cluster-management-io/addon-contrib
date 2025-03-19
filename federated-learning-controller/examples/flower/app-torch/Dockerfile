# Use a smaller base image
FROM python:3.11-slim

# Set environment variables to reduce image size and improve pip behavior
ENV PYTHONUNBUFFERED=1 \
  PIP_NO_CACHE_DIR=1 \
  PIP_DISABLE_PIP_VERSION_CHECK=1 \
  PIP_DEFAULT_TIMEOUT=100 \
  MPLCONFIGDIR=/tmp \
  HF_HOME=/app/.cache/huggingface \
  TRANSFORMERS_CACHE=/app/.cache/transformers

# Set the working directory
WORKDIR /app

# Copy only necessary files (excluding pyproject.toml to defer dependency installation)
COPY . /app

# Ensure the cache directories have the correct permissions
RUN chown -R root:root /app && chmod -R 777 /app


# Set the PYTHONPATH environment variable
ENV PYTHONPATH=/app

# Ensure the entrypoint script is executable
RUN chmod +x /app/entrypoint.sh

# Install dependencies at runtime
# ENTRYPOINT ["sh", "-c", "pip install -e . && /app/entrypoint.sh"]
ENTRYPOINT ["/app/entrypoint.sh"]
