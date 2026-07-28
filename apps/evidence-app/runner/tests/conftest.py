"""Shared test setup for the runner unit tests.

`ASGARDEO_ORG` is a required setting with no default (see `config.py`), so it
must be present in the environment before anything imports `wso2_runner.config`
for the first time — importing that module constructs a `RunnerSettings()` at
module top. Seeding it here lets the happy-path tests construct settings with
`_env_file=None` without each one having to set the org itself; the
missing-org case removes it explicitly with monkeypatch.
"""
import os

os.environ.setdefault("ASGARDEO_ORG", "test-org")
