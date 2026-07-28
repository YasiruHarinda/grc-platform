from pydantic import BaseModel


class ProductCreate(BaseModel):
    name: str
    description: str | None = None


class ProductUpdate(BaseModel):
    name: str | None = None
    description: str | None = None


class ProductResponse(BaseModel):
    id: int
    name: str
    description: str | None

    model_config = {"from_attributes": True}
