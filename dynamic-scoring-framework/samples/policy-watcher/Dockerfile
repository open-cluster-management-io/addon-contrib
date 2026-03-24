FROM python:3.12-slim

WORKDIR /app

# Install uv
RUN pip install --no-cache-dir uv

# Copy project files
COPY main.py .
COPY pyproject.toml .

# Create virtual environment and install dependencies
RUN uv venv && \
    uv sync

# Ensure the virtual environment is in PATH
ENV PATH="/app/.venv/bin:$PATH"


CMD ["python", "-u", "main.py"]