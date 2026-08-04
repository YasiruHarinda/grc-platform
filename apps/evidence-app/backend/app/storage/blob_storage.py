import logging
import uuid
from collections.abc import Iterable
from datetime import datetime, timedelta, timezone
from pathlib import Path
from urllib.parse import urlparse

from azure.core.exceptions import ResourceNotFoundError
from azure.storage.blob import BlobSasPermissions, BlobServiceClient, generate_blob_sas
from fastapi import HTTPException, UploadFile

from app.config import settings

logger = logging.getLogger(__name__)

# How long a signed link stays valid after it's generated. Chosen per ADR
# 0003: long enough to browse a gallery of evidence without a link dying
# mid-view, short enough that a leaked link is quickly useless.
SIGNED_URL_EXPIRY_MINUTES = 15

# Largest evidence upload `save_file` will accept, in bytes. Both the
# Engineer upload path and the Runner (screenshots) only ever produce a
# single image, so 15 MB comfortably covers a high-resolution screenshot
# while still bounding how much any one upload can cost in storage and
# request time.
MAX_UPLOAD_SIZE_BYTES = 15 * 1024 * 1024  # 15 MB

# Content types `save_file` will accept, deliberately an allow-list rather
# than a deny-list (OWASP File Upload Cheat Sheet): evidence and screenshots
# are always images, so only the image types the app actually expects are
# let through, instead of trying to enumerate every dangerous type that
# must be blocked.
ALLOWED_UPLOAD_CONTENT_TYPES = {
    "image/png",
    "image/jpeg",
    "image/webp",
    "image/gif",
    "application/pdf",
}

_blob_service: BlobServiceClient | None = None


def _get_blob_service() -> BlobServiceClient:
    global _blob_service
    if _blob_service is None:
        _blob_service = BlobServiceClient.from_connection_string(
            settings.AZURE_STORAGE_CONNECTION_STRING
        )
    return _blob_service


def save_file(file: UploadFile, *, prefix: str = "", label: str | None = None) -> tuple[str, str]:
    """Upload an evidence file to blob storage and return (blob_name, public_url).

    `prefix` and `label` are both optional and both default to producing
    exactly the old behaviour -- a bare `{uuid}{ext}` -- so every existing
    caller that doesn't pass them is unaffected. When given, the blob name
    becomes `{prefix}{label}-{uuid}{ext}`: `prefix` is meant to be a
    `product/framework/control/`-shaped path (see
    `app.storage.blob_paths.build_control_prefix`) and `label` a short,
    already-sanitised descriptive name (see
    `app.storage.blob_paths.sanitize_title`). This function does no
    sanitisation itself -- a caller passing an unsanitised `label` or
    `prefix` gets exactly that in the blob name, slashes and all -- so
    building a safe hierarchical name is entirely the caller's job; this is
    just string assembly plus the upload.

    Rejects a file with no name at all (`filename` is `None` or empty
    string) as a bad request rather than storing it. The stored blob name
    always ends in a fresh UUID; the client's filename is only ever
    consulted for its extension, so a present-but-unusual name (no
    extension, a trailing dot) still works unchanged. A *missing* name is
    different: a browser file input always attaches one, so in practice
    this only happens with a non-browser client (the Runner, curl, a
    script) that built its request incorrectly. Silently storing it under a
    generated, extensionless name would hide that mistake rather than
    surface it, so it is rejected instead, the same way this app already
    rejects other malformed input up front rather than accepting or masking
    it.

    Also rejects, before any network call, a content type outside the image
    allow-list, or actual bytes over `MAX_UPLOAD_SIZE_BYTES` -- so a
    rejected upload never puts a blob into storage that would then need
    cleaning up. The size check reads the stream itself rather than
    trusting a client-supplied `Content-Length` header, which can lie or be
    absent; it reads at most one byte past the cap, so an oversized upload
    is caught without ever buffering the whole thing into memory.
    """
    if not file.filename:
        raise HTTPException(
            status_code=400,
            detail="Uploaded file must have a filename",
        )
    if file.content_type not in ALLOWED_UPLOAD_CONTENT_TYPES:
        raise HTTPException(
            status_code=400,
            detail=(
                f"Unsupported file type: {file.content_type or 'unknown'!r}. "
                f"Allowed types: {', '.join(sorted(ALLOWED_UPLOAD_CONTENT_TYPES))}"
            ),
        )

    contents = file.file.read(MAX_UPLOAD_SIZE_BYTES + 1)
    if len(contents) > MAX_UPLOAD_SIZE_BYTES:
        raise HTTPException(
            status_code=413,
            detail=(
                "Uploaded file exceeds the "
                f"{MAX_UPLOAD_SIZE_BYTES // (1024 * 1024)} MB limit"
            ),
        )

    extension = Path(file.filename).suffix
    name_stub = f"{label}-" if label else ""
    unique_name = f"{prefix}{name_stub}{uuid.uuid4()}{extension}"
    blob = _get_blob_service().get_blob_client(
        container=settings.AZURE_STORAGE_CONTAINER, blob=unique_name
    )
    blob.upload_blob(contents, overwrite=True)
    return unique_name, f"/uploads/{unique_name}"


def delete_file(file_name: str) -> None:
    """Delete a blob by name. Missing blobs are treated as already deleted."""
    blob = _get_blob_service().get_blob_client(
        container=settings.AZURE_STORAGE_CONTAINER, blob=file_name
    )
    try:
        blob.delete_blob()
    except ResourceNotFoundError:
        pass


def delete_files(file_names: Iterable[str]) -> None:
    """Delete a batch of blobs, e.g. everything under a Control/Framework/
    Product/Evidence being cascade-deleted, with each file's deletion
    handled independently.

    Every delete handler in the app calls this only *after* its database
    commit has already succeeded, so by the time this runs the rows are
    gone for good -- a storage error here must not turn a delete that
    already happened into a failed response, and one file's failure must
    not stop the others from being attempted. So each `delete_file` call is
    isolated: a failure is logged, with the file name (the only remaining
    handle on what is now an orphaned blob), and the loop moves on. Nothing
    here retries -- that's deliberately out of scope, see spec issue #24.

    A missing blob is not a failure: `delete_file` already treats
    `ResourceNotFoundError` as an already-deleted no-op, so it never
    reaches the `except` below and is never logged.
    """
    for file_name in file_names:
        try:
            delete_file(file_name)
        except Exception:
            logger.error(
                "Failed to delete blob %r from storage after its database "
                "row was already committed as deleted; it may now be "
                "orphaned in storage.",
                file_name,
                exc_info=True,
            )


def get_signed_url(file_ref: str, expiry_minutes: int = SIGNED_URL_EXPIRY_MINUTES) -> str:
    """Convert a stored file reference into a freshly generated, time-limited
    Azure SAS URL that points directly at the blob (see ADR 0003).

    `file_ref` is whatever is stored on `Evidence`/`EvidenceFile.file_url`
    today — the `"/uploads/{blob_name}"` form `save_file` returns — but a
    bare blob name is also accepted defensively. The stored value itself is
    never touched; this only runs at read/serialization time, so existing
    rows need no migration.

    Idempotent: a stored file reference is always relative (`/uploads/...`
    or a bare blob name), so a value that already carries a scheme
    (`https://...`) is, by construction, an already-signed URL, not a stored
    reference, and is returned unchanged. Without this, handing an
    already-signed URL back in would strip nothing (there is no
    "/uploads/" prefix to remove), so the whole signed URL — signature,
    expiry and all — would be treated as a blob name and signed again,
    producing a link to a blob that does not exist. Not reachable today
    (FastAPI validates a response model, and therefore signs, exactly
    once), but the same signing call is used from every validator that
    signs a link on the way out, so guarding it here covers all of them.

    `generate_blob_sas` is purely local (no network call) — it only needs
    the account name and account key, both already known from the storage
    connection string.
    """
    if urlparse(file_ref).scheme:
        return file_ref
    blob_name = file_ref.removeprefix("/uploads/")
    service = _get_blob_service()
    blob_client = service.get_blob_client(
        container=settings.AZURE_STORAGE_CONTAINER, blob=blob_name
    )
    sas_token = generate_blob_sas(
        account_name=service.account_name,
        container_name=settings.AZURE_STORAGE_CONTAINER,
        blob_name=blob_name,
        account_key=service.credential.account_key,
        permission=BlobSasPermissions(read=True),
        expiry=datetime.now(timezone.utc) + timedelta(minutes=expiry_minutes),
    )
    return f"{blob_client.url}?{sas_token}"
