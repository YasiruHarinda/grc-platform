"""Unit tests for wso2_runner.env_file — the read/merge/write logic behind
`wso2-runner configure`'s save step.

This is pure string and file handling with no prompting, so it's tested
directly here without driving the CLI wizard. tests/test_cli.py covers the
wizard's end-to-end behaviour (prompts feeding into this module).
"""
import stat

import pytest

import wso2_runner.env_file as env_file
from wso2_runner.env_file import merge_env_lines, read_env_values, write_config_file


# ── merge_env_lines ─────────────────────────────────────────────────────


def test_merge_env_lines_updates_a_key_in_place():
    existing = "AGENT_PROVIDER=azure\nAGENT_MODEL=old\n"

    merged = merge_env_lines(existing, {"AGENT_MODEL": "new"})

    assert merged == ["AGENT_PROVIDER=azure", "AGENT_MODEL=new"]


def test_merge_env_lines_appends_a_new_key_not_already_in_the_file():
    merged = merge_env_lines("AGENT_PROVIDER=azure\n", {"SCREENSHOT_MONITOR": "1"})

    assert merged == ["AGENT_PROVIDER=azure", "SCREENSHOT_MONITOR=1"]


def test_merge_env_lines_preserves_untouched_keys_comments_and_blanks():
    existing = (
        "# hand written notes, do not delete\n"
        "CLOUD_URL=https://cloud.example.com\n"
        "\n"
        "ASGARDEO_ORG=wso2\n"
    )

    merged = merge_env_lines(existing, {"AGENT_PROVIDER": "ollama"})

    assert merged == [
        "# hand written notes, do not delete",
        "CLOUD_URL=https://cloud.example.com",
        "",
        "ASGARDEO_ORG=wso2",
        "AGENT_PROVIDER=ollama",
    ]


def test_merge_env_lines_never_duplicates_an_updated_key():
    existing = "AGENT_PROVIDER=azure\n"

    merged = merge_env_lines(existing, {"AGENT_PROVIDER": "gemini"})

    assert merged.count("AGENT_PROVIDER=gemini") == 1
    assert not any(line.startswith("AGENT_PROVIDER=azure") for line in merged)


# ── read_env_values ──────────────────────────────────────────────────────


def test_read_env_values_returns_empty_dict_when_file_is_missing(tmp_path):
    assert read_env_values(tmp_path / "nope.env") == {}


def test_read_env_values_ignores_comments_and_blank_lines(tmp_path):
    p = tmp_path / ".env"
    p.write_text("# a note\n\nUSER_EMAIL=me@wso2.com\nCLOUD_URL=https://x\n")

    assert read_env_values(p) == {"USER_EMAIL": "me@wso2.com", "CLOUD_URL": "https://x"}


# ── write_config_file ────────────────────────────────────────────────────


def test_write_config_file_creates_a_new_file_with_owner_only_permissions(tmp_path):
    target = tmp_path / ".env"

    write_config_file(target, {"AGENT_PROVIDER": "ollama"})

    assert target.read_text() == "AGENT_PROVIDER=ollama\n"
    mode = stat.S_IMODE(target.stat().st_mode)
    assert mode == 0o600


def test_write_config_file_updates_in_place_and_preserves_the_rest(tmp_path):
    target = tmp_path / ".env"
    target.write_text(
        "# keep me\n"
        "CLOUD_URL=https://cloud.example.com\n"
        "AGENT_PROVIDER=azure\n"
    )

    write_config_file(target, {"AGENT_PROVIDER": "gemini", "GEMINI_API_KEY": "sk-x"})

    content = target.read_text()
    lines = content.splitlines()
    assert "# keep me" in lines
    assert "CLOUD_URL=https://cloud.example.com" in lines
    assert content.count("AGENT_PROVIDER=") == 1
    assert "AGENT_PROVIDER=gemini" in lines
    assert "GEMINI_API_KEY=sk-x" in lines


def test_write_config_file_does_not_leave_a_temp_file_behind(tmp_path):
    target = tmp_path / ".env"

    write_config_file(target, {"AGENT_PROVIDER": "ollama"})

    leftovers = [p for p in tmp_path.iterdir() if p != target]
    assert leftovers == []


def test_write_config_file_leaves_the_original_untouched_if_the_write_fails(tmp_path, monkeypatch):
    """Proves the atomic-write claim: if something goes wrong before the
    rename, the file on disk must still be the old, complete version, not a
    truncated one, and no temp file should be left littering the directory."""
    target = tmp_path / ".env"
    target.write_text("AGENT_PROVIDER=azure\n")

    def boom(*args, **kwargs):
        raise RuntimeError("disk full")

    monkeypatch.setattr(env_file.os, "replace", boom)

    with pytest.raises(RuntimeError):
        write_config_file(target, {"AGENT_PROVIDER": "gemini"})

    assert target.read_text() == "AGENT_PROVIDER=azure\n"
    assert list(tmp_path.iterdir()) == [target]
