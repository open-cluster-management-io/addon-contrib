from pydantic import BaseModel
from typing import List, Dict


class TimeSeries(BaseModel):
    metric: Dict[str, str]
    values: List[List[float]]


class ScoringPayload(BaseModel):
    data: List[TimeSeries]


class Score(BaseModel):
    metric: Dict[str, str]
    score: float


class ScoringResponse(BaseModel):
    results: List[Score]
