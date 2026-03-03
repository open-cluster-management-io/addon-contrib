from fastapi import FastAPI, Request
import uvicorn
from schemas.config import ConfigResponse, ScoringConfig, ScoringParams, SourceConfig
from schemas.scoring import Score, ScoringResponse

app = FastAPI()


@app.get("/healthz")
async def healthcheck():
    return {"status": "ok"}


@app.post("/performance/scoring", response_model=ScoringResponse)
async def performance_scoring(request: Request):
    results = [
        Score(metric={"app": "app01", "device": "all"}, score=80),
        Score(metric={"app": "app02", "device": "all"}, score=80),
        Score(metric={"app": "app01", "device": "3g.48gb"}, score=60),
        Score(metric={"app": "app02", "device": "3g.48gb"}, score=55),
        Score(metric={"app": "app01", "device": "2g.24gb"}, score=10),
        Score(metric={"app": "app02", "device": "2g.24gb"}, score=15),
    ]
    return ScoringResponse(results=results)


@app.get(
    "/performance/config",
    response_model=ConfigResponse,
    response_model_exclude_none=True,
)
async def get_performance_scoring_config():
    return ConfigResponse(
        name="example-performance-scorer",
        description="An example performance score",
        source=SourceConfig(type="None"),
        scoring=ScoringConfig(
            path="/performance/scoring",
            params=ScoringParams(
                name="example_performance_score",
                interval=30,
            ),
        ),
    )


@app.post("/powerconsumption/scoring", response_model=ScoringResponse)
async def powerconsumption_scoring(request: Request):
    results = [
        Score(metric={"app": "app01", "device": "all"}, score=20),
        Score(metric={"app": "app02", "device": "all"}, score=10),
        Score(metric={"app": "app01", "device": "3g.48gb"}, score=30),
        Score(metric={"app": "app02", "device": "3g.48gb"}, score=55),
        Score(metric={"app": "app01", "device": "2g.24gb"}, score=80),
        Score(metric={"app": "app02", "device": "2g.24gb"}, score=90),
    ]
    return ScoringResponse(results=results)


@app.get(
    "/powerconsumption/config",
    response_model=ConfigResponse,
    response_model_exclude_none=True,
)
async def get_powerconsumption_scoring_config():
    return ConfigResponse(
        name="example-powerconsumption-scorer",
        description="An example power consumption score",
        source=SourceConfig(type="None"),
        scoring=ScoringConfig(
            path="/powerconsumption/scoring",
            params=ScoringParams(
                name="example_powerconsumption_score",
                interval=30,
            ),
        ),
    )


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8000)
