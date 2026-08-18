import asyncio
from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import PlainTextResponse

from app.api.routes import products, frameworks, controls, evidence, submissions, agent, usage, me
from app.config import settings


@asynccontextmanager
async def lifespan(app: FastAPI):
    # agent.py's SSE handlers (runner_progress/runner_result) run in
    # FastAPI's threadpool and need a reference to this loop to safely hand
    # updates back to the asyncio.Queue objects that stream_task awaits on.
    # Captured here, while the loop is running, per app.api.routes.agent.
    agent.set_event_loop(asyncio.get_running_loop())
    yield


app = FastAPI(
    title="Compliance Evidence Portal",
    version="1.0.0",
    redirect_slashes=False,
    lifespan=lifespan,
)

cors_origins = [o.strip() for o in settings.CORS_ORIGINS.split(",") if o.strip()]

app.add_middleware(
    CORSMiddleware,
    allow_origins=cors_origins,
    allow_methods=["*"],
    allow_headers=["*"],
    # CORS hides every response header from the page except a short safe-list,
    # and Content-Disposition is not on it. The evidence zip download reads
    # that header to name the file, so without this the browser cannot see it
    # and the archive saves as "evidence-{id}.zip" instead of its real title.
    # Only matters when the webapp is served from its own origin (an absolute
    # BACKEND_BASE_URL, Choreo's React buildpack); harmless when proxied.
    expose_headers=["Content-Disposition"],
)


# Checklist 4.6 mandatory security headers, on every API response. These are
# the safe, additive baseline — none of them restrict where content may load
# from, so nothing the app does is affected. The restrictive CSP directives
# (default-src etc.) and Permissions-Policy are a separate, deferred piece of
# work (see issue #75) — not added here.
#
# The web app's own Strict-Transport-Security value (webapp/index.js) also
# carries `preload`; the API's deliberately does not — see issue #72.
SECURITY_HEADERS = {
    "X-Content-Type-Options": "nosniff",
    "Content-Security-Policy": "upgrade-insecure-requests",
    "Strict-Transport-Security": "max-age=31536000; includeSubDomains",
}


@app.middleware("http")
async def security_headers(request, call_next):
    response = await call_next(request)
    response.headers.update(SECURITY_HEADERS)
    return response


# The middleware above only sees responses the app actually returned. An
# unhandled exception never gets that far: it propagates past this middleware
# to Starlette's ServerErrorMiddleware, which sits outside it and builds the
# 500 itself, so that response would go out with none of the headers. Owning
# the 500 here closes that hole -- "every response" has to include the ones
# nobody planned for.
#
# This reproduces Starlette's own default 500 body and status exactly, and
# ServerErrorMiddleware still re-raises afterwards, so the traceback is
# logged exactly as before. Nothing about diagnosing a crash changes.
@app.exception_handler(Exception)
async def unhandled_exception_handler(request, exc: Exception) -> PlainTextResponse:
    return PlainTextResponse(
        "Internal Server Error",
        status_code=500,
        headers=SECURITY_HEADERS,
    )

# The unauthenticated GET /uploads/{filename} route that used to stream blobs
# straight out of private storage has been removed — evidence files are now
# served via short-lived signed Azure URLs generated at read time
# (app/storage/blob_storage.py:get_signed_url). See ADR 0003.

app.include_router(products.router, prefix="/api")
app.include_router(frameworks.router, prefix="/api")
app.include_router(controls.router, prefix="/api")
app.include_router(evidence.router, prefix="/api")
app.include_router(submissions.router, prefix="/api")
app.include_router(agent.router, prefix="/api")
app.include_router(usage.router, prefix="/api")
app.include_router(me.router, prefix="/api")


@app.get("/health")
def health_check():
    return {"status": "ok"}
