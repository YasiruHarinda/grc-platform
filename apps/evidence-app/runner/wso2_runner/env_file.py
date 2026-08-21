"""Reading, merging, and saving ~/.wso2-runner/.env.

`wso2-runner configure` only ever asks about a handful of settings —
provider, model, that provider's key, the screenshot monitor, the user's
email. Several other settings live in the same file (CLOUD_URL,
ASGARDEO_ORG, ASGARDEO_CLIENT_ID) and are set by hand, once, per the setup
docs, never by the wizard. `configure` must be able to save the settings it
DID ask about without erasing the settings it didn't.

This module is deliberately separate from the wizard in cli.py: it is pure
string and file handling with no prompting, so it can be tested directly
without driving the CLI.
"""

import os
import tempfile
from pathlib import Path


def merge_env_lines(existing_text: str, updates: dict[str, str]) -> list[str]:
    """Merge `updates` into the lines of an existing .env file's text.

    Parsed line by line, not into a plain dict, because a plain dict would
    throw away everything a dict can't represent: comment lines, blank
    lines, and the original position of each setting. A `KEY=VALUE` line
    whose key is in `updates` gets its value swapped in place, on its
    original line, so the key never ends up duplicated. Every other line —
    including a user's hand-added notes — passes through untouched. A key
    in `updates` that isn't already in the file is appended at the end.
    """
    remaining = dict(updates)
    merged_lines = []
    for raw_line in existing_text.splitlines():
        stripped = raw_line.strip()
        if stripped and not stripped.startswith("#") and "=" in stripped:
            key = stripped.split("=", 1)[0].strip()
            if key in remaining:
                merged_lines.append(f"{key}={remaining.pop(key)}")
                continue
        merged_lines.append(raw_line)

    for key, value in remaining.items():
        merged_lines.append(f"{key}={value}")

    return merged_lines


def read_env_values(path: Path) -> dict[str, str]:
    """Read an existing .env file into a plain key -> value dict, ignoring
    comments and blank lines.

    Used only to pre-fill `configure`'s prompts (e.g. showing the current
    USER_EMAIL as the default, so pressing Enter keeps it) — a plain dict is
    fine for that. Saving uses merge_env_lines instead, which keeps the
    file's line structure rather than collapsing it.
    """
    if not path.exists():
        return {}

    values: dict[str, str] = {}
    for raw_line in path.read_text().splitlines():
        stripped = raw_line.strip()
        if not stripped or stripped.startswith("#") or "=" not in stripped:
            continue
        key, _, value = stripped.partition("=")
        values[key.strip()] = value
    return values


def write_config_file(path: Path, updates: dict[str, str]) -> None:
    """Apply `updates` to the .env file at `path`, keeping every other line
    untouched, and save it.

    Written atomically: the merged content is written to a temporary file in
    the same directory first, then moved into place with os.replace(). A
    plain write_text() truncates the target file before writing the new
    bytes, so anything that interrupts the write (a crash, a killed process)
    can leave a half-written or empty .env behind. os.replace() is a single
    filesystem rename, so at every point before it runs the old file is
    still intact, and once it runs the new file is completely in place —
    there is no window where the file on disk is partial. The temp file is
    created in the same directory as `path` so that rename is guaranteed to
    be on the same filesystem (a cross-filesystem "rename" isn't atomic).

    Mode is set to 0600 (owner read/write only) once the file exists. This
    file can hold ANTHROPIC_API_KEY, GEMINI_API_KEY, or AZURE_OPENAI_API_KEY
    in plain text, and the file created by writing to a new path gets
    whatever the process umask allows, which on most machines leaves it
    readable by every other account on the box.
    """
    existing_text = path.read_text() if path.exists() else ""
    merged_lines = merge_env_lines(existing_text, updates)
    content = "\n".join(merged_lines) + "\n"

    fd, tmp_name = tempfile.mkstemp(dir=path.parent, prefix=".env.", suffix=".tmp")
    try:
        with os.fdopen(fd, "w") as f:
            f.write(content)
        os.replace(tmp_name, path)
    finally:
        # Reached only if something above raised before os.replace() ran —
        # in the success path the rename has already consumed tmp_name, so
        # this is a no-op rather than a duplicate delete.
        if os.path.exists(tmp_name):
            os.unlink(tmp_name)

    os.chmod(path, 0o600)
