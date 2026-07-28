from fastapi import APIRouter, Depends

from app.auth import User, get_current_user

router = APIRouter(tags=["Identity"])


@router.get("/me", response_model=User)
async def whoami(user: User = Depends(get_current_user)) -> User:
    return user
