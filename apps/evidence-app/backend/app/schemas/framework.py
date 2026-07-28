from pydantic import BaseModel


class FrameworkCreate(BaseModel):
    product_id: int
    name: str
    description: str | None = None


class FrameworkUpdate(BaseModel):
    name: str | None = None
    description: str | None = None


class FrameworkResponse(BaseModel):
    id: int
    product_id: int
    name: str
    description: str | None

    model_config = {"from_attributes": True}
