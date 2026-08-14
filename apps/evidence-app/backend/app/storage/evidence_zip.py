"""Builds a zip archive of one Evidence's files, laid out so a human can
browse it after extracting (spec issue #92).

Kept out of `blob_storage.py` deliberately, for the same reason
`blob_paths.py` is kept separate from it (see that module's docstring):
`blob_storage.py` talks to the Azure SDK and knows nothing about this app's
domain models, and should stay that way. This module is the domain-aware
half -- it knows about `Evidence`/`EvidenceFile`/`Control` and turns them
into a zip's internal folder names. It reads blob bytes through
`app.storage.blob_storage.read_file`, but never talks to Azure directly, and
it never writes anything to storage.

A zip entry path is presentation, chosen freely when writing the archive --
independent of where the blob actually lives (`blob_paths.py`'s module
docstring: storage has no `evidence` level). So this module is free to add
one:

    With a Control:     product/framework/control/{evidence-title}/NN-{evidence-title}-{uuid}{ext}
    Without a Control:  {evidence-title}/NN-{evidence-title}-{uuid}{ext}

The first three segments (Control case) come from `build_control_prefix`,
reused as-is. `{evidence-title}` is the Evidence's title run through the
existing `sanitize_title` -- the same sanitiser and the same fallback
(`FALLBACK_TITLE_LABEL`) used at upload time, so a title that sanitises to
nothing behaves identically here.

`NN` is the file's 1-based position among the (caller-ordered) files, zero-
padded to 2 digits, or 3 if there are more than 99 -- see
`_entry_number_width`. The `{uuid}{ext}` half of each entry name is pulled
back out of the stored blob's own basename (`_uuid_and_extension`) rather
than reused wholesale, because a Control-filed blob's basename already
contains the upload-time title label (`save_file`'s
`{prefix}{label}-{uuid}{ext}`) -- reusing it verbatim would duplicate the
title in the entry name. Pulling out just the uuid+extension and rebuilding
the label from this module's own `title` gives one consistent entry-name
shape regardless of whether the underlying blob happened to have a label.
"""
import os
import re
import tempfile
import zipfile
from collections.abc import Iterator
from typing import TYPE_CHECKING

from app.storage.blob_paths import build_control_prefix, sanitize_title
from app.storage.blob_storage import read_file

if TYPE_CHECKING:
    from app.models.evidence import Evidence
    from app.models.evidence_file import EvidenceFile

# Matches a canonical uuid.uuid4() string (8-4-4-4-12 hex) plus whatever
# follows it (the extension, if any) at the end of a blob basename. Every
# blob name `save_file` produces ends in exactly one of these -- either
# `{uuid}{ext}` (no prefix/label) or `{label}-{uuid}{ext}` -- so anchoring
# on the uuid shape itself, rather than splitting on "-", is what lets this
# survive a label that itself contains hyphens.
_UUID_AND_EXTENSION_RE = re.compile(
    r"[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}.*$"
)


def _entry_number_width(file_count: int) -> int:
    """2 digits normally, 3 once a collection exceeds 99 files -- see the
    spec's zip-layout section."""
    return 3 if file_count > 99 else 2


def _uuid_and_extension(blob_name: str) -> str:
    """The `{uuid}{ext}` tail of a stored blob name, with any
    `product/framework/control/` path and upload-time title label stripped
    off. Falls back to the bare basename in the (should-never-happen) case
    a blob name doesn't contain a recognisable uuid, rather than raising --
    a defensive fallback, not an expected path."""
    basename = blob_name.rsplit("/", 1)[-1]
    match = _UUID_AND_EXTENSION_RE.search(basename)
    return match.group(0) if match else basename


# Past this much the archive spills to a temporary file on disk instead of
# staying in memory. A single upload is capped at 15 MB
# (`blob_storage.MAX_UPLOAD_SIZE_BYTES`) but the number of files on one
# Evidence is not capped, so an agent collection can total far more than any
# one file. Holding all of that per concurrent download would put the ceiling
# on worker memory rather than on disk, where it belongs.
_SPOOL_MAX_BYTES = 16 * 1024 * 1024

# Size of each chunk handed to the response while streaming the archive back.
_STREAM_CHUNK_BYTES = 64 * 1024


def build_evidence_zip(
    evidence: "Evidence", files: list["EvidenceFile"]
) -> tempfile.SpooledTemporaryFile:
    """Build a zip of `files`, which the caller must have already ordered by
    `sort_order` -- this function trusts that order and does not re-sort, so
    entry `01` is whichever file is first in `files`.

    Returns an open temporary file positioned at the start. Ownership passes
    to the caller, which must close it; `stream_archive` below does that and
    is the intended way to consume the result.

    Callers are responsible for checking `files` is non-empty first: an
    empty zip is a confusing success, not something this function decides
    to allow or reject (see the route).
    """
    title = sanitize_title(evidence.title)
    folder = f"{build_control_prefix(evidence.control)}{title}" if evidence.control_id else title
    width = _entry_number_width(len(files))

    buffer = tempfile.SpooledTemporaryFile(max_size=_SPOOL_MAX_BYTES)
    with zipfile.ZipFile(buffer, mode="w", compression=zipfile.ZIP_DEFLATED) as archive:
        for position, evidence_file in enumerate(files, start=1):
            entry_name = (
                f"{folder}/{position:0{width}d}-{title}-"
                f"{_uuid_and_extension(evidence_file.file_name)}"
            )
            archive.writestr(entry_name, read_file(evidence_file.file_name))

    buffer.seek(0)
    return buffer


def archive_size(archive: tempfile.SpooledTemporaryFile) -> int:
    """Byte length of a built archive, leaving it positioned back at the
    start. Lets the response still send a Content-Length, so the browser
    shows a real progress bar rather than an unknown-length download."""
    size = archive.seek(0, os.SEEK_END)
    archive.seek(0)
    return size


def stream_archive(archive: tempfile.SpooledTemporaryFile) -> Iterator[bytes]:
    """Yield the archive a chunk at a time and close it when done, including
    when the client disconnects part-way through and the response closes the
    generator early."""
    try:
        while chunk := archive.read(_STREAM_CHUNK_BYTES):
            yield chunk
    finally:
        archive.close()
