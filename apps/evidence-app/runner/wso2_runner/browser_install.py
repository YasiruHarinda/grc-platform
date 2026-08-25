"""Making sure Playwright's Chromium is on disk before the Runner opens a
browser with it.

`runner/install.sh` used to cover this as its own step, running
`playwright install chromium` right after installing the package. That
script is being retired in favour of installing the Runner as a plain
wheel, and a package installer (`uv tool install`, `pip install`) has no
way to fetch a browser binary -- it only ever installs Python code. Without
this module, a fresh install would succeed and then fail the first time a
task actually opens a browser, with a confusing error from deep inside
Playwright rather than a plain one at start-up.

This module is deliberately separate from cli.py, the same way env_file.py
is: it is pure detection/subprocess handling with no prompting, so it can
be tested directly -- with Playwright and subprocess.run stubbed -- without
driving the CLI or touching the network.

## The BROWSER_CHANNEL nuance

`settings.BROWSER_CHANNEL` (see config.py) defaults to "chrome", and
`agent.py`'s `_get_browser()` / `doctor`'s launch check both treat any value
other than the literal string "chromium" as "launch a *system* browser via
this channel name" -- "chrome" means the machine's own Google Chrome,
"msedge" would mean the machine's own Microsoft Edge, and so on. Only the
literal channel "chromium" means "launch Playwright's own bundled binary",
which is the one thing `playwright install chromium` actually fetches.

So "the browser is missing" means something different depending on the
channel:

- channel == "chromium": missing means Playwright's bundled binary isn't on
  disk. This module can fix that -- it's exactly what `playwright install
  chromium` is for.
- channel == anything else: the browser in question is a system install
  (Chrome, Edge, ...) that this module has no business managing. Running
  `playwright install chromium` in that case would download something the
  Runner isn't even going to launch, and reporting it as "the browser is
  now installed" would be actively misleading -- Chrome would still be
  missing. Auto-installing a full desktop browser (which may need a system
  package manager and elevated permissions) is out of scope here; the
  default of "chrome" leans on the very common case that a WSO2 engineer's
  laptop already has Chrome, and `doctor`'s existing browser-launch check
  already reports a clear failure if that assumption doesn't hold.

For every channel other than "chromium", this module treats the browser as
already present and does nothing -- not even importing Playwright -- which
is also why the check costs nothing on the common "chrome" path.
"""

import subprocess
import sys
from pathlib import Path

# What `install_chromium()` actually runs, and the command named back to the
# engineer when an automatic install fails. Defined once so the message
# below and the real subprocess call can never drift apart.
MANUAL_INSTALL_COMMAND = "playwright install chromium"

# The one channel value that means "Playwright's own bundled binary" rather
# than a system browser. Named once, because both the detection below and
# the launch-failure advice at the bottom turn on it, and a literal repeated
# in two places is exactly how the two drift apart.
BUNDLED_CHROMIUM_CHANNEL = "chromium"

# Proper product names for the system-browser channels, so a failure message
# reads "Microsoft Edge" rather than "msedge". Any channel not listed here
# falls back to its own name, which is the honest thing to print when we do
# not know what it is called.
_CHANNEL_DISPLAY_NAMES = {
    "chrome": "Google Chrome",
    "chrome-beta": "Google Chrome Beta",
    "msedge": "Microsoft Edge",
    "msedge-beta": "Microsoft Edge Beta",
}

# Printed before the download starts, never after -- a multi-second (or,
# on a slow link, multi-minute) silence with no explanation looks exactly
# like a hang, and the natural reaction to a hang is to kill the process.
DOWNLOAD_WARNING = (
    "\n[runner] Downloading Chromium for the browser agent (about 150 MB) "
    "-- this can take a minute on a slow connection.\n"
)


def _normalise_channel(channel: str) -> str:
    """The channel as the rest of this module compares it.

    Falls back to the same default as `settings.BROWSER_CHANNEL`, so a blank
    or missing value is read as "chrome" (a system browser) rather than
    accidentally as Chromium.
    """
    return (channel or "chrome").strip().lower()


class ChromiumInstallError(Exception):
    """Raised when Chromium needed installing and the attempt failed.

    Carries only the underlying reason (a non-zero exit's stderr, or the
    subprocess failing to start at all) -- naming MANUAL_INSTALL_COMMAND to
    the engineer is left to the caller (cli.py), the same way other start-up
    guards in cli.py compose their own final message around a plain
    exception from a lower-level module (see e.g. how `start` handles
    azure_credential's CredentialUnavailableError). That keeps the "here's
    what to type" phrasing in one place rather than duplicated wherever this
    error might be raised from.
    """


def chromium_is_installed(channel: str) -> bool:
    """Return whether the browser the Runner is actually going to launch
    with `channel` is already on disk.

    Only "chromium" (Playwright's bundled build) is ever checked for real;
    every other channel is reported as installed unconditionally. See the
    module docstring for why -- in short, this module only ever manages
    Playwright's own bundled Chromium, never a system browser.

    Detection asks Playwright itself for the path it would launch, via
    `BrowserType.executable_path`, rather than shelling out to `playwright
    --version` or similar and parsing text -- Playwright already knows
    exactly where it expects its own binary to be, and a plain
    `Path.exists()` on that answer can't be fooled by output-format
    changes across Playwright versions.
    """
    normalised = _normalise_channel(channel)
    if normalised != BUNDLED_CHROMIUM_CHANNEL:
        return True

    try:
        from playwright.sync_api import sync_playwright
    except ImportError:
        # Playwright itself isn't even installed as a Python package yet
        # (shouldn't happen once it's a declared dependency, but a half
        # finished environment is exactly the case this module exists to
        # recover from) -- there is certainly no browser binary either.
        return False

    try:
        with sync_playwright() as p:
            return Path(p.chromium.executable_path).exists()
    except Exception:
        # Any other failure here (a broken Playwright driver process, a
        # corrupted install, ...) is treated the same as "not installed":
        # install_chromium() below is about to try a real
        # `playwright install chromium`, which is the right next step for
        # a broken install too, not just a missing one.
        return False


def install_chromium() -> None:
    """Run the real `playwright install chromium` download.

    Uses `sys.executable -m playwright` -- the same Python interpreter
    running the Runner -- rather than a bare `playwright` on PATH, so this
    always installs into the environment the Runner itself will load
    Playwright from, even if some other `playwright` happens to be on PATH
    (a different venv, a stale global install, ...).

    Raises ChromiumInstallError on any failure -- a non-zero exit from the
    install itself, or the subprocess never starting at all (e.g.
    `playwright` isn't even resolvable as a module). Never returns partial
    success silently: callers can treat "no exception" as "the browser is
    now really there".
    """
    try:
        result = subprocess.run(
            [sys.executable, "-m", "playwright", "install", "chromium"],
            capture_output=True,
            text=True,
        )
    except Exception as exc:
        raise ChromiumInstallError(f"could not run the Chromium installer ({exc})") from exc

    if result.returncode != 0:
        detail = result.stderr.strip() or result.stdout.strip() or f"exit code {result.returncode}"
        raise ChromiumInstallError(f"Chromium install failed: {detail}")


def ensure_chromium_installed(channel: str) -> None:
    """Check for Chromium and install it if `chromium_is_installed(channel)`
    says it's missing.

    This is the one function cli.py calls -- from `configure`, and again as
    a guard on `start` before the poll loop (and therefore any browser)
    ever starts. Printing DOWNLOAD_WARNING happens here, unconditionally,
    before install_chromium() is even attempted, so it's on screen whether
    the install then succeeds or fails -- see DOWNLOAD_WARNING's own
    comment for why that ordering matters.

    Raises ChromiumInstallError on failure, exactly as install_chromium()
    does; callers are expected to catch it and print it rather than let it
    propagate into a browser launch that would fail again anyway, later,
    with a far less clear error.
    """
    if chromium_is_installed(channel):
        return

    print(DOWNLOAD_WARNING)
    install_chromium()


def launch_failure_advice(channel: str) -> str:
    """What to tell an engineer whose browser would not launch, phrased for
    the channel they are actually configured to use.

    `doctor` used to print `playwright install chromium` for every channel.
    That is only correct on the "chromium" channel. On every other channel
    the Runner launches a *system* browser (see the module docstring), and
    the bundled Chromium that command fetches is not the binary that failed
    to launch -- so the engineer downloads roughly 150 MB, waits, and finds
    the original failure exactly where they left it. Wrong advice is worse
    than no advice here, because it costs real time before it is discovered
    to be wrong.

    For a system channel the fix is to install that browser. The message
    also names BROWSER_CHANNEL=chromium as the way out, because someone who
    cannot install Chrome (no admin rights, a locked-down machine) is not
    actually stuck: switching the channel makes the Runner fetch and use its
    own browser instead. That escape hatch is invisible otherwise.
    """
    normalised = _normalise_channel(channel)
    if normalised == BUNDLED_CHROMIUM_CHANNEL:
        return f"Try: {MANUAL_INSTALL_COMMAND}"
    name = _CHANNEL_DISPLAY_NAMES.get(normalised, normalised)
    return (
        f"The Runner is set to the {normalised} channel, which means this "
        f"machine's own {name}.\n"
        f"Install {name}, or set BROWSER_CHANNEL=chromium to have the Runner "
        "download and use its own browser instead."
    )
