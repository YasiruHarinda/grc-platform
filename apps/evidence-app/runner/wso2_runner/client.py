import mimetypes
from pathlib import Path

import httpx

from wso2_runner import oauth


class CloudClient:
    """HTTP client that talks to the cloud backend's task queue API.

    Authenticates every request with a real Asgardeo access token (obtained
    via oauth.get_access_token), the same way the web frontend does — no
    Runner-specific Personal Access Token involved.
    """

    def __init__(self, base_url: str, asgardeo_org: str, asgardeo_client_id: str):
        self.base = base_url.rstrip("/")
        self._org = asgardeo_org
        self._client_id = asgardeo_client_id
        self._http = httpx.AsyncClient(timeout=httpx.Timeout(30.0, read=120.0))

    def _auth_headers(self) -> dict:
        # Cheap in the common case: reads a small local cache file and
        # returns immediately unless the token is actually due for refresh.
        token = oauth.get_access_token(self._org, self._client_id)
        return {"Authorization": f"Bearer {token}"}

    async def get_next_task(self, runner_id: str) -> dict | None:
        r = await self._http.get(
            f"{self.base}/api/agent/tasks/next",
            params={"runner_id": runner_id},
            headers=self._auth_headers(),
        )
        r.raise_for_status()
        return r.json()  # None when queue is empty

    async def heartbeat(self, task_id: int | None = None) -> bool:
        """Tell the backend we're alive while executing a task (we pause polling
        /tasks/next during execution, so without this the UI shows us offline).
        Returns True if the task was cancelled from the UI, so we can stop."""
        r = await self._http.post(
            f"{self.base}/api/agent/heartbeat",
            params={"task_id": task_id} if task_id is not None else None,
            headers=self._auth_headers(),
        )
        r.raise_for_status()
        return bool(r.json().get("cancelled"))

    async def post_progress(self, task_id: int, progress: dict) -> None:
        r = await self._http.post(
            f"{self.base}/api/agent/tasks/{task_id}/progress",
            json=progress,
            headers=self._auth_headers(),
        )
        r.raise_for_status()

    async def post_result(self, task_id: int, result: dict) -> None:
        r = await self._http.post(
            f"{self.base}/api/agent/tasks/{task_id}/result",
            json=result,
            headers=self._auth_headers(),
        )
        r.raise_for_status()

    async def pause_poll(self, task_id: int) -> dict:
        """While paused at a {PAUSE} step, ask the backend whether the user has
        clicked Resume (and whether the task was cancelled). Returns
        {"resume_requested": bool, "cancelled": bool}."""
        r = await self._http.post(
            f"{self.base}/api/agent/tasks/{task_id}/pause-poll",
            headers=self._auth_headers(),
        )
        r.raise_for_status()
        return r.json()

    async def upload_screenshot(self, local_path: Path) -> tuple[str, str]:
        """Upload an evidence file (screenshot PNG or PDF) to backend.
        Returns (file_url, file_name)."""
        content_type = mimetypes.guess_type(local_path.name)[0] or "application/octet-stream"
        with open(local_path, "rb") as f:
            r = await self._http.post(
                f"{self.base}/api/agent/upload-screenshot",
                files={"file": (local_path.name, f, content_type)},
                headers=self._auth_headers(),
            )
        r.raise_for_status()
        data = r.json()
        return data["file_url"], data["file_name"]

    async def aclose(self) -> None:
        await self._http.aclose()
