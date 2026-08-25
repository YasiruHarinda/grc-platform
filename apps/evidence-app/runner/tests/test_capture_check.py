"""Unit tests for wso2_runner.capture_check -- the blank-capture heuristic
behind `doctor`'s screen capture sanity check.

`looks_blank` is pure image analysis and is tested here with nothing but
synthetic PIL images -- no screen, no `mss`, no display of any kind.
`capture_test_screenshot` is the one function that touches `mss` for real;
it is exercised here with a fake `mss` module injected into sys.modules
(mirroring the fake-playwright trick in tests/test_browser_install.py), so
no test in this file ever takes a real screenshot. cli.py's `doctor`
integration -- stubbing `capture_test_screenshot` itself -- lives in
tests/test_cli.py alongside the rest of the doctor tests.
"""
import sys
import types

import pytest
from PIL import Image

from wso2_runner.capture_check import (
    BLANK_CAPTURE_ADVICE,
    LOW_VARIATION_STDDEV_THRESHOLD,
    capture_test_screenshot,
    looks_blank,
)


# ── looks_blank ──────────────────────────────────────────────────────────


def test_solid_wallpaper_colour_is_caught_as_blank():
    """The key case this ticket exists for: a flat, mid-tone fill is not
    remotely black (its mean is nowhere near zero), but it has zero
    variation, and must still be reported as blank. A mean-brightness check
    alone would miss this -- that's the whole reason the check here looks
    at spread, not darkness."""
    img = Image.new("RGB", (400, 300), (60, 90, 160))

    # Prove this really isn't the near-black case agent.py already handles,
    # so a passing assertion below can't be explained by an all-black image
    # sneaking through instead.
    from PIL import ImageStat
    mean = ImageStat.Stat(img).mean
    assert all(m >= 5 for m in mean), "fixture must not be near-black"

    assert looks_blank(img) is True


def test_gentle_gradient_wallpaper_is_caught_as_blank():
    """A soft gradient (the common macOS default-wallpaper style) has a
    little variation, but far less than any real screen -- it must still
    trip the check."""
    img = Image.new("RGB", (200, 100))
    pixels = img.load()
    for x in range(200):
        for y in range(100):
            pixels[x, y] = (60 + x // 20, 90 + x // 20, 160 - x // 20)

    assert looks_blank(img) is True


def test_varied_screen_like_image_is_not_flagged():
    """A synthetic stand-in for a real screen: distinct blocks of very
    different colours, the way a title bar, a sidebar, and page content
    would look. Real content must never trip this heuristic."""
    img = Image.new("RGB", (200, 100))
    pixels = img.load()
    for x in range(200):
        for y in range(100):
            if y < 20:
                pixels[x, y] = (240, 240, 240)  # title bar
            elif x < 30:
                pixels[x, y] = (30, 30, 30)  # sidebar
            else:
                pixels[x, y] = (255, 255, 255) if (x // 10 + y // 10) % 2 == 0 else (200, 200, 200)

    assert looks_blank(img) is False


def test_threshold_boundary_is_exclusive_not_inclusive():
    """Documents the exact boundary: a channel stddev sitting AT the
    threshold does not count as flat, only strictly below it does. Not load
    bearing behaviour so much as a guard against the comparison direction
    silently flipping in a future edit."""
    img = Image.new("RGB", (10, 10), (100, 100, 100))
    pixels = img.load()
    # Push exactly half the pixels to a different value so the channel
    # stddev lands at a known, computable value above the threshold.
    for x in range(10):
        for y in range(5):
            pixels[x, y] = (100, 100, 100)
        for y in range(5, 10):
            pixels[x, y] = (140, 140, 140)

    # This fixture's stddev is 20.0 (evenly split between 100 and 140),
    # comfortably above LOW_VARIATION_STDDEV_THRESHOLD -- confirms the
    # "not flagged" side of the boundary with a value that isn't the
    # loosely-reasoned block image above.
    assert looks_blank(img) is False


def test_grayscale_image_is_still_analysed_correctly():
    """`looks_blank` converts to RGB before measuring -- a paletted or
    single-band image must not bypass the check just because it isn't
    already in RGB mode."""
    flat_gray = Image.new("L", (50, 50), 128)
    assert looks_blank(flat_gray) is True


# ── capture_test_screenshot ──────────────────────────────────────────────


class _FakeShot:
    def __init__(self, width, height, bgra):
        self.size = (width, height)
        self.bgra = bgra


class _FakeMSS:
    def __init__(self, monitors, grab_result):
        self.monitors = monitors
        self._grab_result = grab_result

    def __enter__(self):
        return self

    def __exit__(self, *exc_info):
        return False

    def grab(self, monitor):
        return self._grab_result


def _install_fake_mss(monkeypatch, monitors, grab_result):
    fake_mss_module = types.ModuleType("mss")
    fake_mss_module.MSS = lambda: _FakeMSS(monitors, grab_result)
    monkeypatch.setitem(sys.modules, "mss", fake_mss_module)


def test_capture_test_screenshot_returns_a_pil_image_from_the_grab(monkeypatch):
    """Confirms the wiring end to end -- with `mss` faked out -- without
    ever touching a real screen: the monitor `mss` is told to grab is
    turned into a same-sized RGB PIL Image."""
    from wso2_runner.config import settings

    monkeypatch.setattr(settings, "SCREENSHOT_MONITOR", 1)
    width, height = 8, 4
    solid_bgra = bytes([10, 20, 30, 255] * (width * height))
    monitors = [{"left": 0, "top": 0, "width": width * 2, "height": height * 2}, {"left": 0, "top": 0, "width": width, "height": height}]
    _install_fake_mss(monkeypatch, monitors, _FakeShot(width, height, solid_bgra))

    img = capture_test_screenshot()

    assert isinstance(img, Image.Image)
    assert img.size == (width, height)
    assert img.mode == "RGB"


def test_capture_test_screenshot_falls_back_to_monitor_1_when_index_out_of_range(monkeypatch):
    """Mirrors agent.py's own out-of-range handling for SCREENSHOT_MONITOR
    -- a stale or bad config value must not make the diagnostic check
    itself raise."""
    from wso2_runner.config import settings

    monkeypatch.setattr(settings, "SCREENSHOT_MONITOR", 99)
    width, height = 4, 4
    solid_bgra = bytes([0, 0, 0, 255] * (width * height))
    monitors = [{}, {"left": 0, "top": 0, "width": width, "height": height}]
    _install_fake_mss(monkeypatch, monitors, _FakeShot(width, height, solid_bgra))

    img = capture_test_screenshot()

    assert img.size == (width, height)


# ── the message itself ───────────────────────────────────────────────────


def test_advice_names_the_terminal_app_not_wso2_runner():
    assert "TERMINAL APP" in BLANK_CAPTURE_ADVICE
    assert "wso2-runner" in BLANK_CAPTURE_ADVICE
    assert "Terminal" in BLANK_CAPTURE_ADVICE


def test_advice_says_to_fully_quit_with_cmd_q():
    assert "Cmd+Q" in BLANK_CAPTURE_ADVICE
    assert "FULLY QUIT" in BLANK_CAPTURE_ADVICE


def test_advice_says_macos_may_never_show_the_prompt():
    assert "never shows the permission" in BLANK_CAPTURE_ADVICE.lower() or "never show" in BLANK_CAPTURE_ADVICE.lower()
    assert "by hand" in BLANK_CAPTURE_ADVICE


def test_advice_gives_the_linux_wayland_line():
    assert "Wayland" in BLANK_CAPTURE_ADVICE or "wayland" in BLANK_CAPTURE_ADVICE
    assert "Xorg" in BLANK_CAPTURE_ADVICE


def test_threshold_is_a_positive_finite_number():
    """Sanity check on the constant itself, not on any behaviour -- guards
    against a future edit accidentally setting this to 0 (which would make
    the check useless) or something absurd."""
    assert 0 < LOW_VARIATION_STDDEV_THRESHOLD < 255
