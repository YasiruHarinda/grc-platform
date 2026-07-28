import asyncio
from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

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
