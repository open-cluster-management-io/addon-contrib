import os
import requests
from jinja2 import Environment, FileSystemLoader

VLLM_ENDPOINT = os.getenv(
    "VLLM_ENDPOINT", "http://hosted-llm.example.com/v1/completions"
)
MODEL_NAME = os.getenv(
    "MODEL_NAME", "/models/saved_model_Meta-Llama-3.1-70B-Instruct-FP8"
)

env = Environment(loader=FileSystemLoader("templates"))
template = env.get_template("request_evaluation.j2")


def run_inference(prompt: str) -> float:
    payload = {
        "model": MODEL_NAME,
        "prompt": prompt,
        "temperature": 0.7,
        "max_tokens": 64,
    }
    response = requests.post(VLLM_ENDPOINT, json=payload)
    response.raise_for_status()

    content = response.json()["choices"][0]["text"]
    print(f"Model response: {content.strip()}")

    # Extract the first numeric value from the response
    import re

    match = re.search(r"-?\d+(\.\d+)?", content)
    if not match:
        raise ValueError("No numeric value found in the model response.")

    return float(match.group(0))


if __name__ == "__main__":
    context = "This is a customer number for a retail store. January 5, 2023 is a discount day, and we can expect about twice as many customers as usual."
    timeseries = [
        ("2023-01-01", 75.2),
        ("2023-01-02", 100.5),
        ("2023-01-03", 69.1),
        ("2023-01-04", 82.3),
    ]
    prompt_text = template.render(context=context, timeseries=timeseries)

    prediction = run_inference(prompt_text)
    print(f"Predicted next value: {prediction}")
