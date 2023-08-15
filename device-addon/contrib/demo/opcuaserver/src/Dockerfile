FROM python:3.11-slim

WORKDIR /opcua-sever

COPY ./requirements.txt /opcua-sever
COPY ./server.py /opcua-sever

RUN python -m pip install --no-cache-dir --upgrade -r requirements.txt

CMD ["python", "./server.py"]
