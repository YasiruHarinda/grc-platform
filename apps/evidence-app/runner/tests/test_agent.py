"""Unit tests for the pure helpers in `wso2_runner.agent`.

`agent.py` does `from browser_use import ...` at module top and browser_use
is not installed in this test environment, which is why the rest of the suite
treats the module as untestable. It is not untestable — it just needs the
import satisfied. A placeholder `browser_use` carrying the five names that
import asks for is enough to load the real file, and everything below it that
does not touch a browser is then reachable.

Two decisions in the loading below are load-bearing.

**The module is loaded under a private name, not as `wso2_runner.agent`.**
`tests/test_loop.py` injects a *fake* `wso2_runner.agent` into `sys.modules`
at import time and leaves it there for the rest of the session. Reading or
writing that key here would make these two files fight: whichever imported
second would win, and the loser would fail with an error pointing nowhere
near the cause. Loading straight from the file under `_agent_under_test`
means neither file can see the other's module at all, so collection order
stops mattering. `pytest tests/test_agent.py` alone, the whole suite, and any
future renaming all behave the same.

**The placeholder only has to satisfy the import statement.** Nothing here
calls into browser_use; these are string and arithmetic helpers. If someone
later adds a name to that `from browser_use import ...` line, this file needs
the same name adding below, and the failure will be an ImportError naming it.

Only helpers that touch neither the browser nor the network are covered here.
The capture and pager-walking paths still have no automated coverage, and
still rest on manual runs against the real consoles.
"""
import importlib.util
import sys
import types
from pathlib import Path

import pytest

_AGENT_PATH = Path(__file__).resolve().parent.parent / "wso2_runner" / "agent.py"


def _load_agent_module():
    """Load agent.py under a name nothing else uses.

    Deliberately not `importlib.import_module("wso2_runner.agent")`: that
    consults `sys.modules`, which test_loop.py may already have filled with
    its fake. This bypasses the cache entirely and never writes to the
    `wso2_runner.agent` key.
    """
    placeholder = types.ModuleType("browser_use")
    for name in ("ActionResult", "Agent", "BrowserProfile", "BrowserSession", "Tools"):
        setattr(placeholder, name, type(name, (), {}))

    saved = sys.modules.get("browser_use")
    sys.modules["browser_use"] = placeholder
    try:
        spec = importlib.util.spec_from_file_location("_agent_under_test", _AGENT_PATH)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module
    finally:
        # Put the environment back exactly as it was, so a real browser_use
        # on a developer machine is not left shadowed for later tests.
        if saved is None:
            del sys.modules["browser_use"]
        else:
            sys.modules["browser_use"] = saved


agent = _load_agent_module()


# --- _first_http_url --------------------------------------------------
#
# This decides whether a "PDF:" step runs a model or opens its link
# directly. A wrong answer is not a crash: a link trimmed too eagerly opens
# a page that does not exist, and the capture files that as evidence with no
# agent present to notice. Silent, wrong evidence is the failure this
# function exists to prevent, so its edges are worth pinning.


@pytest.mark.parametrize(
    "text, expected",
    [
        # A bare link, and the whitespace a real prompt collects around it.
        ("https://github.com/o/r/issues/61", "https://github.com/o/r/issues/61"),
        ("  https://github.com/o/r/issues/61  ", "https://github.com/o/r/issues/61"),
        ("\thttps://github.com/o/r/issues/61\n", "https://github.com/o/r/issues/61"),
        ("http://example.com/a", "http://example.com/a"),

        # Prose around the link. This is how the real prompts are written, and
        # the reason the strict "text is only a link" rule was replaced.
        (
            "Open https://github.com/o/r/issues/61, expand all comments, then export as PDF",
            "https://github.com/o/r/issues/61",
        ),
        (
            "Go to https://github.com/o/r/issues/61 and capture it",
            "https://github.com/o/r/issues/61",
        ),

        # Trailing sentence punctuation. Without trimming, the full stop
        # travels into the address and the run captures a 404.
        ("PDF: https://github.com/o/r/issues/61.", "https://github.com/o/r/issues/61"),
        ("PDF: https://github.com/o/r/issues/61,", "https://github.com/o/r/issues/61"),
        ("PDF: https://github.com/o/r/issues/61;", "https://github.com/o/r/issues/61"),

        # Brackets cut both ways, which is why the rule counts rather than
        # strips. A wrapping paren is punctuation; one the link opened itself
        # is part of the address.
        ("(see https://example.com/a)", "https://example.com/a"),
        (
            "https://en.wikipedia.org/wiki/Berlin_(disambiguation)",
            "https://en.wikipedia.org/wiki/Berlin_(disambiguation)",
        ),
        (
            "(https://en.wikipedia.org/wiki/Berlin_(disambiguation))",
            "https://en.wikipedia.org/wiki/Berlin_(disambiguation)",
        ),

        # First link wins: a step prints one page, so a second link cannot
        # change the outcome and must not be able to steal it.
        (
            "Open https://example.com/first then https://example.com/second",
            "https://example.com/first",
        ),

        # No link at all -- the caller reads this as "use the agent".
        ("PDF: the billing settings page", ""),
        ("", ""),
        ("   ", ""),
    ],
)
def test_first_http_url_extracts_the_link_a_pdf_step_should_open(text, expected):
    assert agent._first_http_url(text) == expected


def test_first_http_url_tolerates_none():
    """`execute_task` reads the step text out of a dict, so a missing key
    arrives here as None rather than as a string."""
    assert agent._first_http_url(None) == ""


# --- _is_azure_portal -------------------------------------------------
#
# Every Azure-specific behaviour in this module is gated on this one
# predicate -- the pager walk, the scroll-position capture, the extra tools.
# A false positive would apply Azure handling to AWS or GitHub, which is the
# regression those changes were carefully written to avoid.


@pytest.mark.parametrize(
    "url, expected",
    [
        ("https://portal.azure.com/#browse/Microsoft.Network%2FvirtualNetworks", True),
        ("https://portal.azure.com/", True),
        ("https://console.aws.amazon.com/s3/buckets", False),
        ("https://github.com/o/r/issues/61", False),
        ("", False),
        (None, False),
    ],
)
def test_is_azure_portal_only_matches_the_azure_portal(url, expected):
    assert agent._is_azure_portal(url) is expected
