from fastapi import APIRouter

from app.config import settings

router = APIRouter(tags=["Runner"])


@router.get("/runner-config")
def get_runner_config() -> dict[str, str]:
    """Deliberately unauthenticated -- do not add a dependency on
    get_current_user here.

    The Runner needs its Asgardeo org and client ID *before* it can perform
    an Asgardeo login, so at the point it calls this it has no token yet to
    present. There is no way to reorder that: a request cannot carry
    credentials for the very login it hasn't done yet. `/health` in
    main.py is the existing precedent for a route that has to work this way.

    Neither value handed back here is a secret, which is what makes serving
    them without auth acceptable rather than merely convenient:

    - `asgardeo_client_id` names a public PKCE native client, a type of
      Asgardeo application that has no client secret by design. Knowing the
      ID lets you start a login, not skip one -- Asgardeo still requires an
      interactive sign-in before it issues a token.
    - `asgardeo_org` is no more sensitive than that ID. The web app already
      ships its own client ID (a sibling value of the same kind) to every
      browser that loads it, in plain JS.

    Widening this response is a separate decision each time, not a
    refactor: anything added here inherits "no auth required", so it must be
    re-justified the same way the two fields above are. In particular, the
    Azure OpenAI endpoint/deployment/tenant the Runner also needs are not
    backend settings today and do not belong here -- `configure` asks the
    user for those directly.
    """
    return {
        "asgardeo_org": settings.ASGARDEO_ORG,
        "asgardeo_client_id": settings.ASGARDEO_RUNNER_CLIENT_ID,
    }
