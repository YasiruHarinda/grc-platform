from datetime import datetime
from pydantic import BaseModel


class UsageSummary(BaseModel):
    total_runs: int
    total_input_tokens: int
    total_output_tokens: int
    total_tokens: int
    total_llm_calls: int
    total_cost_usd: float

    last_7_days_cost_usd: float
    last_7_days_tokens: int
    last_7_days_runs: int

    last_30_days_cost_usd: float
    last_30_days_tokens: int
    last_30_days_runs: int

    today_cost_usd: float
    today_runs: int


class UsageDayPoint(BaseModel):
    date: str
    input_tokens: int
    output_tokens: int
    cost_usd: float
    runs: int


class UsageByModel(BaseModel):
    model: str
    runs: int
    input_tokens: int
    output_tokens: int
    total_tokens: int
    cost_usd: float


class UsageLogRow(BaseModel):
    id: int
    run_id: str
    model: str
    provider: str
    input_tokens: int
    output_tokens: int
    total_tokens: int
    llm_calls: int
    cost_usd: float
    subtask_count: int
    created_at: datetime

    model_config = {"from_attributes": True}
