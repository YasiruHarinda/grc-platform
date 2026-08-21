"""Shared test setup for the runner unit tests.

`ASGARDEO_ORG` now defaults to "" (see `config.py`) — it no longer has to be
present for `RunnerSettings()`, or importing `wso2_runner.config`, to
succeed. It's still seeded here because the process-wide `settings`
singleton (built once, the first time anything imports `wso2_runner.config`)
is shared by every test in the suite, and `cli.py`'s `start`/`doctor` now
gate on ASGARDEO_ORG being non-empty. Without this, every pre-existing CLI
test that has nothing to do with Asgardeo would trip that gate just because
the host running the tests has no such variable set. Tests that specifically
exercise the missing-org case remove it explicitly with monkeypatch.
"""
import os

os.environ.setdefault("ASGARDEO_ORG", "test-org")
