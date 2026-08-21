"""Whether the one test screenshot `doctor` takes actually captured anything.

Screen capture goes through `mss` (see agent.py's real capture path, used
for every piece of evidence the Runner uploads). On macOS, without Screen
Recording permission, `mss` does not raise -- macOS quietly hands back the
desktop wallpaper, or a plain black image, instead of failing the call. The
Runner then happily uploads that as "evidence", with no error anywhere in
the pipeline. The failure only ever shows up days later, as "my evidence is
all blank", in someone else's review of the customer's screenshots -- and
it reads as a bug in the tool rather than a missing OS permission. Linux
has the same trap under Wayland, which recent Ubuntu ships as the default
session.

A fully black capture is already caught in agent.py's real screenshot path
-- see its near-zero MEAN brightness check, which recovers by falling back
to a Chrome CDP screenshot instead. That check does not catch a wallpaper:
a wallpaper is not black, and its mean brightness can be almost anything.
What a wallpaper (or a bare desktop, or any other single flat capture) has
in common with a black screen is not brightness -- it's the *absence of
variation*. Every pixel is close to every other pixel, because nothing
resembling a window, text, or a browser is actually on screen. So this
module checks spread, not darkness.

This module exists to run that check once, on demand, from `doctor`'s own
diagnostic pass, independent of agent.py's runtime recovery path. It is
deliberately separate from cli.py, the same way env_file.py and
browser_install.py are: `looks_blank` is pure image analysis with no
capture and no printing, so it can be tested with synthetic PIL images and
no screen at all; `capture_test_screenshot` is the only place that touches
`mss` for real, so it is the only thing a test needs to stub out. Printing
the result -- the ✓/✗ line and the fix-it message -- is left to `doctor`
in cli.py, matching how it already composes every other check's output.

## This is a heuristic, not a permission check

There is no cross-platform API that answers "does this process have Screen
Recording permission" -- the only signal available at all is what the
capture actually came back with. That cuts both ways: a machine that
legitimately shows a blank desktop, or a genuinely plain solid-colour
background, as its real screen will trip this too. Because a false alarm is
possible, this check must only ever warn. `doctor` keeps exiting zero when
it fires, and nothing here is ever called from `start` -- a real permission
problem still fails loudly the moment a task tries to upload a screenshot,
which is a pre-existing, unrelated code path this ticket does not touch.
"""

from PIL import Image, ImageStat

# Below this per-channel standard deviation, an image is treated as "close
# enough to a single flat colour" to call blank. Picked from measuring
# actual inputs, not guessed:
#
#   - a solid colour fill (a bare desktop, or a permission-less capture on
#     some Linux desktops): stddev == 0.0
#   - a smooth wallpaper-style gradient (the common macOS default
#     wallpaper style): stddev around 3
#   - any real application window -- text, icons, a taskbar, a browser's
#     own chrome -- always has hard edges between very different colours
#     somewhere in the frame: stddev in the high tens at the very least,
#     usually much higher
#
# 10 sits comfortably above the flat/gradient cluster and comfortably below
# the real-content cluster, so an ordinary screen (never perfectly uniform)
# does not trip it, while a wallpaper or a bare desktop reliably does.
LOW_VARIATION_STDDEV_THRESHOLD = 10.0

# The three details that actually get people unstuck, per the ticket this
# module was written for -- each one is a place engineers get stuck without
# it: granting the permission to the wrong app, closing the window instead
# of quitting it, and waiting forever for a prompt macOS never shows for a
# CLI tool.
BLANK_CAPTURE_ADVICE = """\
      This usually means the OS handed back a blank desktop or wallpaper
      instead of the real screen. Evidence captured this way will be
      blank too, with nothing else in the tool to say so.

      macOS: grant Screen Recording permission to your TERMINAL APP
      (Terminal, iTerm2, VS Code, ...), not to wso2-runner -- wso2-runner
      is a command running inside that app, so it will not appear in the
      permission list at all. macOS often never shows the permission
      prompt for a CLI tool on its own, so add it by hand: System Settings
      > Privacy & Security > Screen Recording. Then FULLY QUIT the
      terminal with Cmd+Q -- closing the window is not enough, the app has
      to stop running -- and reopen it before trying again.

      Linux: this can happen under Wayland. Run `echo $XDG_SESSION_TYPE`;
      if it says "wayland", sign out and choose an Xorg session (often
      listed as "Ubuntu on Xorg") at the login screen instead."""


def looks_blank(img: Image.Image) -> bool:
    """Return whether `img` has almost no pixel-to-pixel variation.

    Converted to RGB first so this works the same for a paletted or
    greyscale image as it does for the RGB frames `capture_test_screenshot`
    actually produces -- `ImageStat.Stat.stddev` is computed per band, and
    a mode with a different band count shouldn't change what "flat" means.

    Checked against every channel's standard deviation, not just one: a
    flat capture is flat in all three, but requiring "any channel" instead
    of "all channels" would also catch a real screen that happens to be
    monochrome-ish in one channel (a mostly-red or mostly-blue UI), which
    is real content, not a blank capture.
    """
    stat = ImageStat.Stat(img.convert("RGB"))
    return all(channel_stddev < LOW_VARIATION_STDDEV_THRESHOLD for channel_stddev in stat.stddev)


def capture_test_screenshot() -> Image.Image:
    """Take one real screen capture with `mss`, for `doctor` to hand to
    `looks_blank`.

    Uses the same monitor index the Runner would actually capture during a
    real task (`settings.SCREENSHOT_MONITOR`) rather than always grabbing
    the primary monitor -- this check is only useful if it tests the same
    capture path the Runner relies on for real evidence. Picking *which*
    monitor is right, on a multi-monitor machine, is a separate concern
    (see chala2001/grc-tools#101); this function just reads whatever is
    already configured, the same way agent.py's real capture does.

    Deliberately minimal: no cropping, no compression, no fallback to
    gnome-screenshot the way agent.py's real capture path has -- this is a
    diagnostic probe of the first, most common capture method, not a
    second implementation of evidence capture. `import mss` happens here,
    lazily, rather than at module load, so importing this module (and
    therefore testing `looks_blank`) never requires `mss` to be installed,
    matching the lazy-import style agent.py already uses for the same
    reason.

    Raises on any failure -- a missing `mss` install, no display available,
    an out-of-range monitor index -- exactly as `capture_test_screenshot`'s
    caller (`doctor`) expects: doctor reports capture failure as its own
    line, separately from a successful-but-blank capture.
    """
    import mss

    with mss.MSS() as sct:
        monitors = sct.monitors
        from wso2_runner.config import settings

        idx = settings.SCREENSHOT_MONITOR
        if idx < 0 or idx >= len(monitors):
            idx = 1
        shot = sct.grab(monitors[idx])
        return Image.frombytes("RGB", shot.size, shot.bgra, "raw", "BGRX")
