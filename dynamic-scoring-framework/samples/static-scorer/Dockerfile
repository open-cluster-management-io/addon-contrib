FROM python:3.11-slim

WORKDIR /app

COPY app ./

RUN pip install --no-cache-dir fastapi uvicorn pydantic

EXPOSE 8000

CMD ["python", "main.py"]