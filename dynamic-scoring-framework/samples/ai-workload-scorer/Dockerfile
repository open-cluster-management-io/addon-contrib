FROM python:3.11-slim

WORKDIR /app

COPY app/main.py ./main.py
COPY app/schemas/ ./schemas/
COPY app/static/ ./static/

RUN mkdir -p /app/models
RUN chmod -R 777 /app/models

RUN pip install --no-cache-dir fastapi uvicorn pydantic darts pandas numpy

EXPOSE 8000

CMD ["python", "main.py"]