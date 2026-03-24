from pydantic import BaseModel
from typing import Optional


class PrometheusParams(BaseModel):
    query: str
    range: int
    step: int


class SourceConfig(BaseModel):
    type: str
    host: Optional[str] = None
    path: Optional[str] = None
    params: Optional[PrometheusParams] = None


class ScoringParams(BaseModel):
    name: str
    interval: int


class ScoringConfig(BaseModel):
    path: str
    params: ScoringParams


class ConfigResponse(BaseModel):
    name: str
    description: str
    source: SourceConfig
    scoring: ScoringConfig
