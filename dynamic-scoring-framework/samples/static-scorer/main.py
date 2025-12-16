from fastapi import FastAPI, Request
import uvicorn

app = FastAPI()


@app.get("/healthz")
async def healthcheck():
    return {"status": "ok"}


@app.post("/performance/scoring")
async def performance_scoring(request: Request):
    results = [
        {
            "metric": {
                "app": "app01",
                "device": "all",
            },
            "score": 80,
        },
        {
            "metric": {
                "app": "app02",
                "device": "all",
            },
            "score": 80,
        },
        {
            "metric": {
                "app": "app01",
                "device": "3g.48gb",
            },
            "score": 60,
        },
        {
            "metric": {
                "app": "app02",
                "device": "3g.48gb",
            },
            "score": 55,
        },
        {
            "metric": {
                "app": "app01",
                "device": "2g.24gb",
            },
            "score": 10,
        },
        {
            "metric": {
                "app": "app02",
                "device": "2g.24gb",
            },
            "score": 15,
        },
    ]
    return {"results": results}


@app.get("/performance/config")
async def get_performance_scoring_config():
    config = {
        "name": "example-performance-scorer",
        "description": "An example performance score",
        "source": {
            "type": "none",
        },
        "scoring": {
            "path": "/performance/scoring",
            "params": {
                "name": "example_performance_score",
                "interval": 30,
            },
        },
    }
    return config


@app.post("/powerconsumption/scoring")
async def powerconsumption_scoring(request: Request):
    results = [
        {
            "metric": {
                "app": "app01",
                "device": "all",
            },
            "score": 20,
        },
        {
            "metric": {
                "app": "app02",
                "device": "all",
            },
            "score": 10,
        },
        {
            "metric": {
                "app": "app01",
                "device": "3g.48gb",
            },
            "score": 30,
        },
        {
            "metric": {
                "app": "app02",
                "device": "3g.48gb",
            },
            "score": 55,
        },
        {
            "metric": {
                "app": "app01",
                "device": "2g.24gb",
            },
            "score": 80,
        },
        {
            "metric": {
                "app": "app02",
                "device": "2g.24gb",
            },
            "score": 90,
        },
    ]
    return {"results": results}


@app.get("/powerconsumption/config")
async def get_powerconsumption_scoring_config():
    config = {
        "name": "example-powerconsumption-scorer",
        "description": "An example power consumption score",
        "source": {
            "type": "none",
        },
        "scoring": {
            "path": "/powerconsumption/scoring",
            "params": {
                "name": "example_powerconsumption_score",
                "interval": 30,
            },
        },
    }
    return config


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8000)
