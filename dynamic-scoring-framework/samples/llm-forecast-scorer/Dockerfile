FROM python:3.11-slim

WORKDIR /app

COPY main.py .
COPY llm-test.py .

RUN mkdir /app/templates
RUN mkdir /app/static
RUN chmod -R 755 /app/templates
RUN chmod -R 755 /app/static
COPY templates /app/templates
COPY static /app/static

RUN pip install --no-cache-dir fastapi uvicorn pydantic requests jinja2

EXPOSE 8000

CMD ["python", "main.py"]