"""
Role-based access checks. Built as FastAPI dependencies so a route can declare
which roles may call it with a single line:

    @router.delete("/products/{id}")
    def delete_product(..., user: User = Depends(require_admin)):
        ...

The role comes from User.role, which today is mapped from an ADMIN_EMAILS
allow-list in auth.py::_role_for. Swap that one function for a real Asgardeo
group/role claim later — no change needed here.
"""
from fastapi import Depends, HTTPException, status

from app.auth import User, get_current_user


def require_role(*allowed_roles: str):
    async def dep(user: User = Depends(get_current_user)) -> User:
        if user.role not in allowed_roles:
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail=f"Role '{user.role}' is not allowed here. Need one of: {', '.join(allowed_roles)}",
            )
        return user

    return dep


require_admin = require_role("admin")
