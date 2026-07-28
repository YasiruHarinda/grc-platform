"""Browser-use agent execution — runs on the local machine where Chromium lives."""

import asyncio
import base64
import io
import json
import re
import time
import uuid
from pathlib import Path
from typing import Awaitable, Callable

from browser_use import ActionResult, Agent, BrowserProfile, BrowserSession, Tools

from wso2_runner.config import settings

# ── Pricing ────────────────────────────────────────────────────────────────

LLM_PRICING: list[tuple[str, tuple[float, float]]] = [
    ("gpt-4o-mini", (0.15, 0.60)),
    ("gpt-4o", (2.50, 10.00)),
    ("gpt-4.1-mini", (0.40, 1.60)),
    ("gpt-4.1", (2.00, 8.00)),
    ("gpt-5-mini", (0.50, 2.00)),
    ("gpt-5", (5.00, 15.00)),
    ("claude-haiku", (1.00, 5.00)),
    ("claude-sonnet", (3.00, 15.00)),
    ("claude-opus", (15.00, 75.00)),
    ("gemini-2.5-flash", (0.075, 0.30)),
    ("gemini-2.0-flash", (0.075, 0.30)),
    ("gemini-flash", (0.075, 0.30)),
    ("gemini", (1.25, 5.00)),
    ("qwen", (0.0, 0.0)),
    ("llama", (0.0, 0.0)),
    ("ollama", (0.0, 0.0)),
]


def _resolve_pricing(model: str) -> tuple[float, float]:
    m = (model or "").lower()
    for key, prices in LLM_PRICING:
        if key in m:
            return prices
    return (0.0, 0.0)


def _compute_cost(input_tokens: int, output_tokens: int, model: str) -> float:
    in_per_m, out_per_m = _resolve_pricing(model)
    return round((input_tokens / 1_000_000) * in_per_m + (output_tokens / 1_000_000) * out_per_m, 6)


# ── Token counter ──────────────────────────────────────────────────────────

class _TokenCounter:
    def __init__(self) -> None:
        self.input_tokens = 0
        self.output_tokens = 0
        self.calls = 0

    def _record(self, result) -> None:
        self.calls += 1
        in_t, out_t = 0, 0
        usage = getattr(result, "usage", None)
        if usage:
            in_t = getattr(usage, "prompt_tokens", 0) or getattr(usage, "input_tokens", 0) or 0
            out_t = getattr(usage, "completion_tokens", 0) or getattr(usage, "output_tokens", 0) or 0
        if not in_t and not out_t:
            meta = getattr(result, "usage_metadata", None)
            if meta:
                d = meta if isinstance(meta, dict) else dict(meta)
                in_t = d.get("input_tokens", 0) or d.get("prompt_tokens", 0) or 0
                out_t = d.get("output_tokens", 0) or d.get("completion_tokens", 0) or 0
        if not in_t and not out_t:
            rm = getattr(result, "response_metadata", None) or {}
            tu = rm.get("token_usage") or rm.get("usage") or {}
            in_t = tu.get("prompt_tokens", 0) or tu.get("input_tokens", 0) or 0
            out_t = tu.get("completion_tokens", 0) or tu.get("output_tokens", 0) or 0
        self.input_tokens += int(in_t or 0)
        self.output_tokens += int(out_t or 0)


def _attach_token_counter(llm) -> _TokenCounter:
    existing = getattr(llm, "_compliance_counter", None)
    if existing is not None:
        return existing
    counter = _TokenCounter()
    orig = getattr(llm, "ainvoke", None)
    if orig:
        async def _tracked(*a, **kw):
            result = await orig(*a, **kw)
            try: counter._record(result)
            except Exception: pass
            return result
        try: llm.ainvoke = _tracked
        except Exception: pass
    try: llm._compliance_counter = counter
    except Exception: pass
    return counter


# ── Subtask parsing ────────────────────────────────────────────────────────

_SUBTASK_RE = re.compile(r"^\s*(?:\d+[.)\-:]?|[-*•►▶→])\s*(.+)$")

# Template markers — a subtask line can stand for "repeat this for every
# discovered item" or "do this for the first and last page" instead of being
# a literal one-shot instruction. See _expand_template_subtask().
_EACH_RE = re.compile(r"^\s*EACH\s*:\s*(.+)$", re.IGNORECASE | re.DOTALL)
_EACH_PAGE_RE = re.compile(r"^\s*EACH-PAGE\s*:\s*(.+)$", re.IGNORECASE | re.DOTALL)

# "PDF:" is not a template (no discovery/expansion) — it's a single literal
# subtask whose CAPTURE step differs: export the page as a PDF (after
# expanding any "Load more" content) instead of the usual scrolling
# screenshots. See _capture_evidence_pdf() and its use in execute_task().
_PDF_RE = re.compile(r"^\s*PDF\s*:\s*(.+)$", re.IGNORECASE | re.DOTALL)

# "FILTER:" deterministically fills a list's own local filter/search box
# (never the page's global search bar) via code, before any LLM is involved
# for that step — see _deterministic_fill_filter(). Falls back to normal
# agent-driven typing if no confident target element is found.
_FILTER_RE = re.compile(r"^\s*FILTER\s*:\s*(.+)$", re.IGNORECASE | re.DOTALL)

# "{PAUSE}" anywhere in a step makes the runner STOP after that step and wait for
# the user to click "Resume" in the UI — a human-in-the-loop break so the user
# can set up complex filtering (e.g. Azure subscription/resource-group filters)
# manually in the browser before the agent reads the resulting list. Stripped
# from the step text; sets a per-subtask `pause` flag. See execute_task/on_pause.
_PAUSE_RE = re.compile(r"\{\s*PAUSE\s*\}", re.IGNORECASE)


def parse_subtasks(prompt: str) -> list[dict]:
    """Split a prompt into subtasks. Returns a list of
    {"text": str, "kind": ..., "pause": bool} dicts, kind one of
    "literal" | "each" | "each_page" | "pdf" | "filter".

    "EACH: <template with {item}>" and "EACH-PAGE: <template with {page}>"
    are template subtasks — expanded at runtime into N literal subtasks by
    _expand_template_subtask() once the item count is known.

    "PDF: <instruction>" is a literal subtask that gets captured as a PDF
    instead of scrolling screenshots.

    "FILTER: <text>" deterministically fills the current page's own local
    list filter/search box with <text> — no LLM call for the common case.

    "{PAUSE}" (anywhere in a step) pauses the run after that step until the user
    clicks Resume, so they can apply filters manually first.
    """
    # Handle inline numbered items typed on one line: "1. do X 2. do Y" → two lines
    normalized = re.sub(r'(?<=\S)\s+(\d+[.)\-:])\s?', r'\n\1 ', prompt.strip())
    lines = normalized.splitlines()
    tasks: list[list[str]] = []
    current: list[str] = []
    for line in lines:
        m = _SUBTASK_RE.match(line)
        if m:
            if current:
                tasks.append(current)
            current = [m.group(1).strip()]
        elif current and line.strip():
            current.append(line.strip())
    if current:
        tasks.append(current)
    joined = ["\n".join(t).strip() for t in tasks if t]
    raw = [j for j in joined if j] or [prompt.strip()]

    specs: list[dict] = []
    for text in raw:
        # Strip a {PAUSE} marker (if present) before kind-matching, and remember it.
        pause = bool(_PAUSE_RE.search(text))
        if pause:
            text = _PAUSE_RE.sub(" ", text).strip()
        m_each = _EACH_RE.match(text)
        m_page = _EACH_PAGE_RE.match(text)
        m_pdf = _PDF_RE.match(text)
        m_filter = _FILTER_RE.match(text)
        if m_each:
            spec = {"text": m_each.group(1).strip(), "kind": "each"}
        elif m_page:
            spec = {"text": m_page.group(1).strip(), "kind": "each_page"}
        elif m_pdf:
            spec = {"text": m_pdf.group(1).strip(), "kind": "pdf"}
        elif m_filter:
            spec = {"text": m_filter.group(1).strip(), "kind": "filter"}
        else:
            spec = {"text": text, "kind": "literal"}
        spec["pause"] = pause
        specs.append(spec)
    return specs


# ── Agent instructions ─────────────────────────────────────────────────────

AGENT_INSTRUCTIONS = """\
CRITICAL AUTHENTICATION RULES — READ FIRST:
- You are already logged in. A human user has already authenticated this browser session.
- DO NOT type any usernames, passwords, MFA codes, or credentials.
- DO NOT click "Sign in", "Log in", or "Continue" on any login page.
- DO NOT invent or guess credentials under any circumstance.
- If you see a login screen, sign-in prompt, or authentication challenge, STOP immediately
  and report: 'NOT_LOGGED_IN — user must authenticate first'.

CRITICAL READ-ONLY RULES — THIS IS A CAPTURE TASK, NOT AN ADMINISTRATION TASK:
- Your only job is to VIEW and NAVIGATE. You are gathering evidence, not making changes.
- DO NOT click any button, link, menu item, or icon whose purpose is to create, edit,
  update, save, delete, disable, enable, approve, reject, terminate, stop, start, restart,
  deploy, publish, rotate, revoke, or otherwise change any resource, setting, permission,
  or configuration — on any cloud console, GitHub, or other site.
- DO NOT submit any form, DO NOT type into any field other than a read-only search/filter
  box you are using to locate the resource you were asked to find.
- Only interact with elements needed to VIEW the target screen: links, menu items, tabs,
  breadcrumbs, pagination ("next"/"previous"), expand/collapse toggles for reading content,
  and search/filter controls used purely to locate something on screen.
- If you are unsure whether clicking something would change state, treat it as if it would
  and do NOT click it. Instead, capture whatever read-only view you can reach and note in
  your final report that you could not confirm an element was safe to click.

SEARCH & NAVIGATION STRATEGY (be smart, not literal):
- When asked to find a resource by name (S3 bucket, Key Vault, VM, etc.):
  1. Try the EXACT name the user gave you first.
  2. If no exact match, try common variations:
     - swap hyphens / underscores / spaces: "cloud-care", "cloud_care", "cloudcare", "cloud care"
     - try lowercase, Title Case, UPPERCASE
     - try with common prefixes/suffixes: "dev-X", "prod-X", "X-bucket", "X-prod"
  3. If still no match, list all available resources and pick the one whose name
     is MOST SIMILAR (case-insensitive substring match, fuzzy match on words).
  4. If there are multiple candidates, pick the most likely one AND screenshot the full list
     so the user can verify.
- For cloud consoles with regions (AWS), if a resource is not found, also try switching
  regions or check the "global" / "all regions" view.
- Always prefer capturing SOMETHING relevant over giving up empty-handed.

SCREENSHOT STRATEGY:
- You do NOT need to take a screenshot yourself. A screenshot is captured automatically
  by the system the moment you call done().
- Your only job is to NAVIGATE to the correct page and ensure it is fully loaded
  (no spinners, content visible), then call done() immediately.
- Do NOT call evaluate(), do NOT check document.readyState, do NOT attempt any
  screenshot action. Just navigate and call done() — the system does the rest.
- If you can't find the exact thing asked for, navigate to the closest related view
  and call done() — the system will capture whatever is on screen.

IF A CLICK OR NAVIGATION ISN'T WORKING:
- To click something, prefer clicking its numbered element directly — the same way you click
  any link or button. You do NOT need find_elements/extract to locate a simple navigation
  target like a menu item, tab, section header, or collapsible group. Those tools are for
  reading/enumerating MULTIPLE items on screen at once, not for finding one thing to click.
- If find_elements/extract for the same target fails, or returns the exact same result, twice
  in a row, STOP retrying selector variations immediately — repeating the same failing
  approach will not suddenly start working and only wastes steps.
- Instead: look at your own current list of numbered clickable elements and find the one whose
  visible text matches what you're looking for, then click it by that number, same as any
  other click. If nothing matches, scroll to reveal more of the page and check again —
  collapsible sections, side-panel menus, and expandable groups often need a real click
  (not a query) to open, and may not even exist in the page's data until expanded.

REPORTING (always in your final answer):
- WHAT YOU WERE ASKED to find or do.
- WHAT YOU ACTUALLY FOUND: exact match, close match, or "nothing similar found".
- ANY VARIATIONS or substitutions you tried.
- ANY BLOCKERS (permission errors, region issues, page didn't load).

TASK TO PERFORM (assume you are already authenticated):
"""


# ── Discovery instructions (for EACH: / EACH-PAGE: template subtasks) ──────

DISCOVERY_EACH_INSTRUCTIONS = """\
CRITICAL AUTHENTICATION RULES — READ FIRST:
- You are already logged in. Do NOT type credentials or interact with login screens.

DISCOVERY MODE — READ CAREFULLY:
- Your ONLY job is to identify every distinct item on this page that matches
  the description below. Do NOT click into any individual item, do NOT
  perform the per-item action described — that happens in a later step.
- If you just applied (or are about to rely on) a search/filter box, WAIT at
  least 1-2 seconds and re-check the results before concluding there are
  none — many consoles (AWS, Azure) debounce search and the table can briefly
  show "0 results" or a loading state right after typing. Never report empty
  results without waiting for the table to finish loading first.
- If the list is paginated or requires scrolling, page/scroll through ALL of
  it before answering, so no items are missed.
- You have VISION enabled for this task. If a text-extraction tool
  (extract/find_elements/CSS selectors) fails to return the item names —
  wrong selector, no matches, or the same failure repeating — do NOT keep
  retrying different selector variations. After at most 2 failed attempts,
  stop and instead read the item names DIRECTLY off the screenshot with
  your own vision. Visually reading a visible list of names is just as
  valid as extracting it, and is required once extraction isn't working —
  getting stuck retrying selectors is the one thing you must not do here.
- When you have the full list, call done() with your final answer being
  ONLY a JSON array of the exact item names/identifiers as they appear on
  screen — nothing else: no prose before or after it, no summary sentence,
  no markdown code fences.
  Example final answer: ["kv-prod-1", "kv-prod-2", "kv-dev-3"]
  If you find none, return [].
- If the list is long (dozens or more items), that changes NOTHING about the
  above — output every single item individually and completely in the JSON
  array regardless of length. NEVER abbreviate, summarize, truncate, or use
  "..." / "and so on" / "through" to represent a range instead of listing
  each one — an incomplete array is worse than a slow one.

WHAT TO ENUMERATE — the {item} placeholder below refers to each item you must find:
"""

DISCOVERY_EACH_PAGE_INSTRUCTIONS = """\
CRITICAL AUTHENTICATION RULES — READ FIRST:
- You are already logged in. Do NOT type credentials or interact with login screens.

DISCOVERY MODE — READ CAREFULLY:
- Your ONLY job is to find the pagination control on this results view and
  determine the total number of pages (the LAST page number). Do NOT click
  into any individual result, do NOT change the current filter/page.
- If you just applied (or are about to rely on) a search/filter box, WAIT at
  least 1-2 seconds and re-check before reading the pagination control —
  many consoles (AWS, Azure) debounce search and briefly show a stale or
  empty state right after typing. Never report "1 page" / "no results"
  without waiting for the table to finish loading first.
- When you know the total, call done() with your final answer being ONLY a
  JSON object of the form {"total_pages": N} — nothing else, no prose.
  If there is genuinely no pagination control or only one page, return
  {"total_pages": 1}.

CONTEXT — the {page} placeholder below refers to the page number you must determine the range for:
"""


def _parse_json_loose(text: str):
    """Extract the first JSON array/object in a possibly-chatty agent answer."""
    text = (text or "").strip()
    try:
        return json.loads(text)
    except Exception:
        pass
    for open_c, close_c in (("[", "]"), ("{", "}")):
        start = text.find(open_c)
        end = text.rfind(close_c)
        if start != -1 and end != -1 and end > start:
            try:
                return json.loads(text[start:end + 1])
            except Exception:
                continue
    return None


async def _expand_template_subtask(
    kind: str,
    template_text: str,
    llm,
    browser: "BrowserSession",
    context_prefix: str,
    max_steps: int,
    base_url: str | None = None,
) -> list[str]:
    """Run a discovery agent pass, then expand a template subtask into
    concrete literal subtask strings.

    kind == "each": discovers a list of item names, substitutes each into
      `template_text` wherever "{item}" appears (or appends the name if the
      placeholder is missing).
    kind == "each_page": discovers the total page count, produces one
      subtask for page 1 and one for the last page (or just page 1 if there's
      only one page), substituting "{page}". Each generated subtask is
      prefixed with an explicit instruction to navigate the pagination
      control to that page first — the browser session persists across
      subtasks, so without this the agent just screenshots whatever page
      it's already sitting on instead of actually moving to it.

    `base_url`, when given, is the URL the browser was on right before this
    template subtask started (captured by the caller). We deterministically
    navigate back to it here, in code, before running discovery — this is
    NOT left to the LLM to remember: an agent that has drifted (e.g. after
    accidentally opening a nav menu) can otherwise "recover" by searching its
    own account/profile for a similarly-named resource and silently
    capturing the wrong one. Re-navigating by URL removes that failure mode
    entirely rather than just asking the model not to do it.

    Discovery always runs with vision ON regardless of the task's own
    use_vision setting: counting/enumerating items is a fundamentally visual
    judgment, and reading it from the raw DOM alone has proven unreliable
    against virtualized table components (e.g. AWS/Azure console grids),
    which can look empty in the DOM even while visibly populated on screen.
    """
    location_note = ""
    if base_url:
        try:
            current_url = await _cdp_eval(browser, "window.location.href")
            if current_url != base_url:
                await browser.navigate_to(base_url)
        except Exception:
            pass
        location_note = _base_url_override_note(base_url)

    if kind == "each":
        # Azure only: read the list's filter term NOW (while it's still applied)
        # so we can keep only items whose NAME contains it. Azure's "Filter for
        # any field" matches every column, so the visible list can include rows
        # that matched on resource group / subscription rather than name.
        name_filter = ""
        items = None
        if _is_azure_portal(base_url):
            name_filter = await _read_list_filter_value(browser)
            # Read the list names in CODE first — the model corrupts/truncates
            # long Azure lists (drops characters, cuts the JSON off), and the
            # resulting names then fail to open. Deterministic read gives exact
            # names. Falls back to the model only if no name links are found.
            det_names = await _read_azure_list_names(browser)
            print(f"[runner]   EACH: filter box reads {name_filter!r}; "
                  f"deterministic read found {len(det_names)} name(s)")
            if det_names:
                items = det_names

        if items is None:
            discovery_task = DISCOVERY_EACH_INSTRUCTIONS + context_prefix + location_note + template_text
            agent = Agent(task=discovery_task, llm=llm, browser=browser, use_vision=True, max_actions_per_step=3)
            history = await agent.run(max_steps=max_steps)
            raw_result = str(history.final_result() or "")
            items = _parse_json_loose(raw_result)
            if not isinstance(items, list):
                print(f"[runner]   EACH: discovery did not return valid JSON, got: {raw_result[:2000]!r}")
                return []
            print(f"[runner]   EACH: model discovery returned {len(items)} item(s)")

        if name_filter:
            nf = name_filter.lower()
            by_name = [n for n in items if nf in str(n).lower()]
            # Only narrow if the term actually appears in some names — otherwise
            # (e.g. the model returned display names that omit the term) keep the
            # original list rather than produce an empty worklist.
            if by_name:
                dropped = len(items) - len(by_name)
                if dropped:
                    print(f"[runner]   EACH: name-filter '{name_filter}' kept {len(by_name)}, "
                          f"dropped {dropped} that matched only in other columns.")
                items = by_name

        return [
            template_text.format(item=name) if "{item}" in template_text
            else f"{template_text} (item: {name})"
            for name in items if str(name).strip()
        ]

    if kind == "each_page":
        discovery_task = DISCOVERY_EACH_PAGE_INSTRUCTIONS + context_prefix + location_note + template_text
        agent = Agent(task=discovery_task, llm=llm, browser=browser, use_vision=True, max_actions_per_step=3)
        history = await agent.run(max_steps=max_steps)
        raw_result = str(history.final_result() or "")
        parsed = _parse_json_loose(raw_result)
        total_pages = 1
        if isinstance(parsed, dict) and isinstance(parsed.get("total_pages"), (int, float)):
            total_pages = max(1, int(parsed["total_pages"]))
        elif not isinstance(parsed, dict):
            print(f"[runner]   EACH-PAGE: discovery did not return valid JSON, defaulting to 1 page. Got: {raw_result[:2000]!r}")
        pages = [1] if total_pages <= 1 else [1, total_pages]
        return [
            (
                f"Use the pagination control (usually at the bottom of the list/table) to navigate to "
                f"page {p} of the results — if you are not already on it. Then: "
                + (template_text.format(page=p) if "{page}" in template_text else f"{template_text} (page: {p})")
            )
            for p in pages
        ]

    return []


# ── Screenshot storage ─────────────────────────────────────────────────────

_SCREENSHOTS_DIR = Path.home() / ".wso2-runner" / "screenshots"
_SCREENSHOTS_DIR.mkdir(parents=True, exist_ok=True)

_PROFILE_DIR = Path.home() / ".wso2-runner" / "browser_profile"
_PROFILE_DIR.mkdir(parents=True, exist_ok=True)


def _save_screenshot_local(raw: str) -> Path:
    if raw.startswith("data:image"):
        raw = raw.split(",", 1)[1]
    path = _SCREENSHOTS_DIR / f"{uuid.uuid4()}.png"
    path.write_bytes(base64.b64decode(raw))
    return path


def _capture_os_screenshot() -> bytes:
    """Capture the screen at OS level. Returns PNG bytes.
    Raises RuntimeError if all methods fail so callers can fall back to CDP.

    Priority:
      1. MSS          — works on macOS, Windows, Linux Xorg
      2. gnome-screenshot with clean env — works on Linux GNOME Wayland
         (VS Code snap pollutes LD_LIBRARY_PATH; stripping it fixes the crash)
    """
    import io
    import os
    import subprocess
    import tempfile
    import mss
    import mss.tools
    from PIL import Image, ImageStat

    def _crop_to_monitor(raw: bytes) -> bytes:
        """Crop a full-desktop capture to the configured monitor's bounding box.
        gnome-screenshot captures all monitors combined — this extracts just the
        right one. Returns original bytes if geometry lookup fails."""
        try:
            with mss.MSS() as sct:
                idx = settings.SCREENSHOT_MONITOR
                if idx < 0 or idx >= len(sct.monitors):
                    return raw
                m = sct.monitors[idx]
                box = (m["left"], m["top"], m["left"] + m["width"], m["top"] + m["height"])
                img = Image.open(io.BytesIO(raw)).convert("RGB")
                img = img.crop(box)
                buf = io.BytesIO()
                img.save(buf, format="PNG", optimize=True)
                return buf.getvalue()
        except Exception:
            return raw

    def _compress(raw: bytes, max_width: int = 1920) -> bytes:
        """Resize to max_width if wider and re-encode as JPEG quality 85.
        JPEG gives 5-10x smaller files than PNG — keeps uploads well under
        any server size limit while staying visually lossless for screenshots."""
        try:
            img = Image.open(io.BytesIO(raw)).convert("RGB")
            if img.width > max_width:
                ratio = max_width / img.width
                img = img.resize((max_width, int(img.height * ratio)), Image.LANCZOS)
            buf = io.BytesIO()
            img.save(buf, format="JPEG", quality=85, optimize=True)
            return buf.getvalue()
        except Exception:
            return raw

    # ── 1. MSS (Mac / Windows / Linux Xorg) ───────────────────────────────
    try:
        with mss.MSS() as sct:
            monitors = sct.monitors  # [0]=all, [1]=primary, [2]=external
            idx = settings.SCREENSHOT_MONITOR
            if idx < 0 or idx >= len(monitors):
                idx = 1
            shot = sct.grab(monitors[idx])
            png = mss.tools.to_png(shot.rgb, shot.size)
        img = Image.open(io.BytesIO(png)).convert("RGB")
        stat = ImageStat.Stat(img)
        if not all(m < 5 for m in stat.mean):
            return _compress(png)  # real pixels — compress and return
    except Exception:
        pass

    # ── 2. gnome-screenshot with snap-clean environment (Linux GNOME Wayland)
    clean_env = {}
    for key in ("HOME", "DISPLAY", "WAYLAND_DISPLAY", "DBUS_SESSION_BUS_ADDRESS",
                "XDG_RUNTIME_DIR", "USER", "LOGNAME", "XDG_SESSION_TYPE"):
        val = os.environ.get(key)
        if val:
            clean_env[key] = val
    clean_env["PATH"] = "/usr/bin:/usr/local/bin:/bin"

    fd, tmp_path = tempfile.mkstemp(suffix=".png")
    os.close(fd)
    tmp = Path(tmp_path)
    try:
        subprocess.run(
            ["gnome-screenshot", "-f", str(tmp)],
            env=clean_env,
            timeout=8, check=True,
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        )
        if tmp.exists() and tmp.stat().st_size > 10_000:
            raw = tmp.read_bytes()
            # gnome-screenshot captures all monitors combined — crop to the right one
            raw = _crop_to_monitor(raw)
            return _compress(raw)
    except Exception:
        pass
    finally:
        tmp.unlink(missing_ok=True)

    raise RuntimeError("All OS screenshot methods failed")


def _add_evidence_header(screenshot_bytes: bytes, url: str, title: str) -> bytes:
    """Draw a browser-chrome-style header bar on top of a CDP screenshot.

    The header shows the page URL, title, timestamp and user email so auditors
    have full context without needing an OS-level screenshot.
    """
    import io
    from datetime import datetime, timezone
    from PIL import Image, ImageDraw, ImageFont

    img = Image.open(io.BytesIO(screenshot_bytes)).convert("RGB")
    w, h = img.size

    BAR_H = 48
    ACCENT_H = 3                          # orange top stripe
    ICON_COLOR = (255, 115, 0)            # WSO2 orange
    BG = (30, 30, 30)                     # dark chrome bar
    TEXT_URL = (220, 220, 220)
    TEXT_META = (150, 150, 150)

    # Try system fonts; PIL default is fine as final fallback
    font_url = font_meta = ImageFont.load_default()
    for path in [
        "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
        "/usr/share/fonts/truetype/liberation/LiberationMono-Regular.ttf",
        "/usr/share/fonts/truetype/ubuntu/Ubuntu-R.ttf",
    ]:
        try:
            font_url = ImageFont.truetype(path, 13)
            font_meta = ImageFont.truetype(path, 11)
            break
        except Exception:
            pass

    # Build header canvas
    header = Image.new("RGB", (w, BAR_H + ACCENT_H), BG)
    draw = ImageDraw.Draw(header)

    # Orange accent stripe at very top
    draw.rectangle([0, 0, w, ACCENT_H - 1], fill=ICON_COLOR)

    # Small filled circle as "browser icon" placeholder
    circle_x, circle_y = 12, ACCENT_H + 14
    draw.ellipse([circle_x, circle_y, circle_x + 12, circle_y + 12], fill=ICON_COLOR)

    # URL text — truncate if too long
    url_x = 34
    max_url_chars = max(40, (w - 320) // 8)
    url_display = url if len(url) <= max_url_chars else url[:max_url_chars - 1] + "…"
    draw.text((url_x, ACCENT_H + 8), url_display, fill=TEXT_URL, font=font_url)

    # Page title (smaller, below URL)
    if title:
        title_display = title if len(title) <= max_url_chars else title[:max_url_chars - 1] + "…"
        draw.text((url_x, ACCENT_H + 26), title_display, fill=TEXT_META, font=font_meta)

    # Timestamp + user on right
    ts = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M:%S UTC")
    user_line = f"{settings.USER_EMAIL}  |  {ts}"
    try:
        meta_w = draw.textlength(user_line, font=font_meta)
    except Exception:
        meta_w = len(user_line) * 7
    draw.text((w - meta_w - 12, ACCENT_H + 16), user_line, fill=TEXT_META, font=font_meta)

    # Thin separator line at bottom of header
    draw.line([0, BAR_H + ACCENT_H - 1, w, BAR_H + ACCENT_H - 1], fill=(60, 60, 60))

    # Combine header + screenshot
    canvas = Image.new("RGB", (w, BAR_H + ACCENT_H + h))
    canvas.paste(header, (0, 0))
    canvas.paste(img, (0, BAR_H + ACCENT_H))

    buf = io.BytesIO()
    canvas.save(buf, format="PNG")
    return buf.getvalue()


async def _cdp_eval(browser: "BrowserSession", expression: str):
    cdp_session = await browser.get_or_create_cdp_session()
    result = await cdp_session.cdp_client.send.Runtime.evaluate(
        params={"expression": expression, "returnByValue": True},
        session_id=cdp_session.session_id,
    )
    return result["result"].get("value")


async def _cdp_sessions(browser: "BrowserSession"):
    """Return (cdp_client, [session_id, ...]) for the top frame plus every
    (incl. cross-origin) iframe. Azure Portal projects each resource's blade
    into cross-origin iframes that the top-frame-only `_cdp_eval` can't see;
    browser-use exposes each as its own CDP target session (get_all_frames)."""
    try:
        main = await browser.get_or_create_cdp_session()
    except Exception:
        return None, []
    client = main.cdp_client
    session_ids = [main.session_id]  # top frame first
    try:
        _all_frames, target_sessions = await browser.get_all_frames()
        for sid in target_sessions.values():
            if sid not in session_ids:
                session_ids.append(sid)
    except Exception:
        pass  # fall back to just the top frame
    return client, session_ids


async def _cdp_eval_session(client, session_id: str, expression: str):
    """Evaluate JS in one specific frame session."""
    try:
        result = await client.send.Runtime.evaluate(
            params={"expression": expression, "returnByValue": True},
            session_id=session_id,
        )
        return result["result"].get("value")
    except Exception:
        return None


async def _cdp_eval_in_frames(browser: "BrowserSession", expression: str):
    """Evaluate JS in EVERY frame and return the FIRST truthy result. Use this
    for "find THE unique element and act" scripts (e.g. click a row by exact
    name) where at most one frame can match. For "find the best-of-several
    candidates" (e.g. the right filter box among multiple search bars), use
    `_run_in_best_frame` instead — first-truthy would wrongly pick a weak match
    in an earlier frame over a better one in a later frame."""
    client, session_ids = await _cdp_sessions(browser)
    if not client:
        return None
    for sid in session_ids:
        value = await _cdp_eval_session(client, sid, expression)
        if value:  # truthy → this frame had the element and acted on it
            return value
    return None


async def _run_in_best_frame(browser: "BrowserSession", score_expr: str, action_expr: str):
    """Score EVERY frame with `score_expr` (a JS snippet returning a number —
    higher = better match) and run `action_expr` only in the highest-scoring
    frame. This fixes the multi-search-bar problem: an Azure page can have a
    global search bar, a left-nav menu filter AND the list's own "Filter for any
    field" box, each in a different frame. First-truthy-frame would grab whatever
    box appears first; scoring every frame and acting only in the best one picks
    the real list filter regardless of frame order. Returns the action result,
    or None if no frame scored above zero."""
    client, session_ids = await _cdp_sessions(browser)
    if not client:
        return None
    best_sid, best_score = None, 0
    for sid in session_ids:
        score = await _cdp_eval_session(client, sid, score_expr)
        if isinstance(score, (int, float)) and score > best_score:
            best_score, best_sid = score, sid
    if not best_sid or best_score <= 0:
        return None
    return await _cdp_eval_session(client, best_sid, action_expr)


async def _wheel_scroll(browser: "BrowserSession", delta_y: float) -> None:
    """Fire real mouse-wheel events via browser-use's own Mouse actor. The
    browser routes each one to whatever is actually scrollable under that
    point — an inner container, a virtualized list, even content inside an
    iframe — exactly like a human scrolling with a mouse. This sidesteps DOM
    inspection entirely, which proved unreliable against console UIs like
    AWS/Azure that nest their real scroll area below the top-level document.

    Fires at three horizontal positions (75%/50%/25% of viewport width,
    each with a third of the requested delta) instead of a single dispatch
    at the exact center: real console layouts are often off-center — e.g.
    AWS's "Amazon Q" assistant panel occupies the left half of the S3
    console, with the actual bucket table on the right, so a single
    center-point wheel event can land on the assistant panel (which doesn't
    scroll) and silently do nothing, making the capture loop think it
    already reached the bottom when it never actually scrolled the content
    that matters. Splitting the delta three ways keeps the total scroll
    distance roughly the same even when more than one position happens to
    land on the same scrollable region."""
    page = await browser.must_get_current_page()
    mouse = await page.mouse
    viewport_w = await _cdp_eval(browser, "window.innerWidth") or 1280
    viewport_h = await _cdp_eval(browser, "window.innerHeight") or 800
    y = viewport_h / 2
    for frac in (0.75, 0.5, 0.25):
        try:
            await mouse.scroll(x=viewport_w * frac, y=y, delta_y=delta_y / 3)
        except Exception:
            pass


# Finds the page's own LOCAL list filter/search box and fills it — never the
# site's GLOBAL search bar. Heuristic: exclude any input inside a <header>/
# role="banner" landmark or within the top ~120px of the viewport (that band
# is almost universally the global nav bar across AWS/Azure/GitHub), prefer
# one whose placeholder mentions "filter"/"search", else take the first
# remaining candidate. This is deliberately positional/landmark-based, not
# tied to any one console's specific class names.
#
# Setting `.value` directly does NOT trigger React's (or similar frameworks')
# internal state update — Azure Portal, like most modern web apps, only
# reacts to a real synthetic `input` event. This uses the native property
# setter (bypassing the framework's overridden one) then dispatches a real
# `input` + `change` event, which is the standard way to fill a
# framework-controlled input from outside the framework.
# Collects every element of a set of tags across the whole document INCLUDING
# open Shadow DOM roots — Azure's Ibiza grid controls (its filter box, row
# links) are rendered inside web components whose content lives in shadow roots,
# which plain document.querySelectorAll does NOT pierce. Combined with
# _cdp_eval_in_frames (which crosses iframe boundaries), this reaches controls
# hidden behind BOTH of Azure's isolation layers.
_SHADOW_WALK_JS = """
  function deepQuery(selector) {
    const out = [];
    const visit = (root) => {
      let nodes;
      try { nodes = root.querySelectorAll(selector); } catch (e) { nodes = []; }
      for (const n of nodes) out.push(n);
      const all = root.querySelectorAll('*');
      for (const el of all) { if (el.shadowRoot) visit(el.shadowRoot); }
    };
    visit(document);
    return out;
  }
"""

# Single source of truth for picking the list's LOCAL filter box, shared by the
# score / fill / read scripts so all three agree on the same element. Returns
# {el, tier} for the best candidate in this frame, or null. Tiers (higher wins):
#   4 = placeholder/aria "filter for any" (Azure's exact list filter)
#   3 = mentions "filter"
#   2 = mentions "search" (but NOT the global "Search resources…" bar)
#   1 = any other plausible text input (last resort)
# Scoring every frame and taking the global max (see _run_in_best_frame) is what
# lets us ignore a page's OTHER search bars — the global one, and the left-nav
# menu filter — and hit the actual list filter, wherever/whatever frame it's in.
# No top-of-viewport guard: inside Azure's blade iframe, coordinates are
# iframe-relative, so the list filter (near the iframe's top) has a tiny y; the
# global bar is excluded by placeholder text instead.
_FILTER_PICK_JS = """
  function pickFilterInput() {
    const inputs = deepQuery('input');
    let bestEl = null, bestTier = 0;
    for (const el of inputs) {
      const type = (el.type || 'text').toLowerCase();
      if (!['text', 'search', ''].includes(type)) continue;
      if (el.closest('header, [role="banner"]')) continue;
      const rect = el.getBoundingClientRect();
      if (rect.width < 60 || rect.height < 8) continue;
      const hay = ((el.placeholder || '') + ' ' + (el.getAttribute('aria-label') || '')).toLowerCase();
      if (hay.includes('search resources') || hay.includes('services, and docs')
          || hay.includes('services and docs')) continue;
      let tier = 1;
      if (hay.includes('filter for any')) tier = 4;
      else if (hay.includes('filter')) tier = 3;
      else if (hay.includes('search')) tier = 2;
      if (tier > bestTier) { bestTier = tier; bestEl = el; }
    }
    return bestEl ? { el: bestEl, tier: bestTier } : null;
  }
"""

# Returns this frame's best filter-box tier (0 if none) — used to compare frames.
_FILTER_SCORE_JS_TEMPLATE = """
(() => {
  __SHADOW__
  __PICK__
  const r = pickFilterInput();
  return r ? r.tier : 0;
})()
"""

# Uses __TEXT__ / __SHADOW__ / __PICK__ tokens (not str.format) because the
# injected snippets contain literal { } that would collide with format fields.
_FIND_AND_FILL_FILTER_JS_TEMPLATE = """
(() => {
  const TEXT = __TEXT__;
  __SHADOW__
  __PICK__
  const r = pickFilterInput();
  if (!r) return false;
  const target = r.el;
  const nativeSetter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set;
  nativeSetter.call(target, TEXT);
  target.dispatchEvent(new Event('input', { bubbles: true }));
  target.dispatchEvent(new Event('change', { bubbles: true }));
  target.focus();
  return true;
})()
"""


async def _deterministic_fill_filter(browser: "BrowserSession", text: str) -> bool:
    """Find the page's local filter/search box (not the global search bar)
    and fill it deterministically, then press Enter to commit it. Returns
    True if a confident target was found and filled, False if nothing
    matched (caller should fall back to normal agent-driven typing)."""
    score_js = (_FILTER_SCORE_JS_TEMPLATE
                .replace("__SHADOW__", _SHADOW_WALK_JS)
                .replace("__PICK__", _FILTER_PICK_JS))
    fill_js = (_FIND_AND_FILL_FILTER_JS_TEMPLATE
               .replace("__SHADOW__", _SHADOW_WALK_JS)
               .replace("__PICK__", _FILTER_PICK_JS)
               .replace("__TEXT__", json.dumps(text)))
    try:
        # Score every frame (top, iframes) and fill only the best-matching frame's
        # filter box — a page can have several search bars (global, left-nav menu
        # filter, list filter) in different frames, so we compare match quality
        # across ALL frames instead of taking whichever appears first.
        filled = await _run_in_best_frame(browser, score_js, fill_js)
    except Exception as exc:
        print(f"[runner]   filter: deterministic fill errored: {exc}")
        return False
    if not filled:
        print(f"[runner]   filter: no local 'Filter for any field' box found in any frame "
              f"for \"{text}\" — agent will be asked to type it manually.")
        return False
    print(f"[runner]   filter: typed \"{text}\" into the list's local filter box deterministically.")
    await asyncio.sleep(0.3)
    try:
        page = await browser.must_get_current_page()
        await page.press("Enter")
    except Exception:
        pass
    await asyncio.sleep(0.6)  # let the list re-render/filter
    return True


# Reads the current text sitting in the list's own filter box (same box
# _deterministic_fill_filter targets). Used to name-filter discovery results:
# Azure's "Filter for any field" matches EVERY column (name, resource group,
# subscription, tags), so a filter of "choreo" also keeps vaults whose NAME
# isn't choreo-* but whose resource group is (e.g. dev-csi-64 in
# choreo-dev-key-vault-rg). Reading the term back lets us keep only rows whose
# NAME actually contains it — the "name only" filter Azure's UI can't do.
_READ_FILTER_VALUE_JS_TEMPLATE = """
(() => {
  __SHADOW__
  __PICK__
  const r = pickFilterInput();
  if (!r) return null;
  return r.el.value || '';
})()
"""


async def _read_list_filter_value(browser: "BrowserSession") -> str:
    """Return the text currently in the list's local filter box, or '' if none
    is found / it's empty. Scores every frame and reads from the best-matching
    filter box (same selection as the fill), so on a page with several search
    bars it reads the real list filter, not the left-nav menu filter."""
    score_js = (_FILTER_SCORE_JS_TEMPLATE
                .replace("__SHADOW__", _SHADOW_WALK_JS)
                .replace("__PICK__", _FILTER_PICK_JS))
    read_js = (_READ_FILTER_VALUE_JS_TEMPLATE
               .replace("__SHADOW__", _SHADOW_WALK_JS)
               .replace("__PICK__", _FILTER_PICK_JS))
    try:
        value = await _run_in_best_frame(browser, score_js, read_js)
    except Exception:
        return ""
    return (value or "").strip() if isinstance(value, str) else ""


# ── Azure left-menu navigation (the deterministic "helper move") ────────────
#
# Azure Portal (Ibiza) projects each resource's UI through cross-origin iframes
# that constantly re-render (virtualized lists, async blade loads). The normal
# perceive→pick-an-index→click loop misfires there because the element the LLM
# picked has been re-numbered by the time the click fires — hence the observed
# "element indexes become stale quickly / clicked an unrelated item" failures.
#
# This finds a left-nav item by its EXACT visible text and clicks it in the
# same JS tick (no gap for the page to shuffle), confined to the left-nav
# column and below the top global-nav band, so it can't hit a same-named tab
# in the main content area (e.g. the "Properties" TAB on Overview vs. the
# "Properties" item under Settings). It is invoked by the LLM as a tool
# (click_menu_item) from natural language — NOT by any user-typed keyword.
_FIND_AND_CLICK_LABEL_JS_TEMPLATE = """
(() => {{
  const normalize = s => (s || '').replace(/\\s+/g, ' ').trim().toLowerCase();
  const TARGET = normalize({label_json});
  const MIN_Y = {min_y_json};

  const all = document.querySelectorAll('body *');
  let exact = null, prefix = null;
  for (const el of all) {{
    if (el.children.length > 2) continue;
    const text = normalize(el.textContent);
    // Prefer an EXACT label; otherwise accept a word-boundary PREFIX so a natural
    // shorthand matches the full menu text (e.g. "CLI" → "CLI / PS", "Access
    // control" → "Access control (IAM)") without loosening into partial-word
    // false matches (e.g. "CLI" must not match "climate").
    let kind = 0;
    if (text === TARGET) kind = 2;
    else if (text.length > TARGET.length && text.startsWith(TARGET)
             && /[^a-z0-9]/.test(text.charAt(TARGET.length))) kind = 1;
    if (!kind) continue;
    const rect = el.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) continue;
    if (rect.left > 480) continue;
    if (rect.top < 120) continue;
    if (MIN_Y !== null && rect.top <= MIN_Y) continue;
    if (kind === 2) {{ if (!exact || rect.top < exact.top) exact = {{ el, top: rect.top }}; }}
    else {{ if (!prefix || rect.top < prefix.top) prefix = {{ el, top: rect.top }}; }}
  }}
  const best = exact || prefix;
  if (!best) return null;
  if ({click_json}) {{
    const clickable = best.el.closest('button, a, li, [role="treeitem"], [role="button"], [tabindex]') || best.el;
    clickable.click();
  }}
  return best.top;
}})()
"""


async def _find_and_click_label(
    browser: "BrowserSession", label: str, min_y: float | None = None, click: bool = True
) -> float | None:
    """Find a left-nav-style label by exact text match and (when click=True)
    click it. Returns the matched element's y-coordinate (so a child search can
    be constrained below it, and so callers can test existence with click=False),
    or None if nothing confidently matched."""
    min_y_json = "null" if min_y is None else repr(float(min_y))
    click_json = "true" if click else "false"
    js = _FIND_AND_CLICK_LABEL_JS_TEMPLATE.format(
        label_json=json.dumps(label), min_y_json=min_y_json, click_json=click_json
    )
    try:
        return await _cdp_eval(browser, js)
    except Exception:
        return None


async def _click_resource_menu_item(browser: "BrowserSession", item: str, section: str = "") -> bool:
    """Deterministically click a left-nav item, optionally inside a collapsible
    section. Clicking a section header TOGGLES it, so we must not click a section
    that's already open — doing so collapses it and triggers an expand/collapse
    loop (the exact "clicks Automation, un-expands, re-expands…" waste seen in
    testing). Instead we locate the section WITHOUT clicking, check whether the
    item is already visible below it (== already expanded), and only click the
    section to expand it when the item isn't showing yet. Then click the item,
    constrained BELOW the section so a same-named label elsewhere can't match.
    Returns True only if the final item click landed."""
    section = (section or "").strip()
    item = (item or "").strip()
    if not item:
        return False

    section_y = None
    if section and section.lower() != item.lower():
        # Locate the section without clicking it.
        section_y = await _find_and_click_label(browser, section, click=False)
        if section_y is not None:
            # Only expand if the item isn't already visible (section collapsed).
            item_y = await _find_and_click_label(browser, item, min_y=section_y, click=False)
            if item_y is None:
                await _find_and_click_label(browser, section, click=True)  # expand it
                await asyncio.sleep(0.7)  # let the group's children render
                # Re-locate the section (its position may shift as children appear).
                section_y = await _find_and_click_label(browser, section, click=False) or section_y

    result = await _find_and_click_label(browser, item, min_y=section_y, click=True)
    if result is None:
        # One retry after a longer settle — the group may still be animating open.
        await asyncio.sleep(0.6)
        result = await _find_and_click_label(browser, item, min_y=section_y, click=True)
    if result is None:
        return False
    await asyncio.sleep(0.5)
    return True


# Opens a row in a LIST (e.g. the Key Vaults grid) by its exact name. Azure list
# rows are dense and near-identical, so picking one by numbered index is exactly
# where the agent opened the wrong vault. This matches the row's link/text
# exactly and clicks it in one tick — preferring a real anchor, and only in the
# main content area (below the top nav band). Exact match avoids opening a
# similarly-named neighbour (e.g. "choreo-customer-prod" vs "choreo-stg-...").
_OPEN_LIST_ITEM_JS_TEMPLATE = """
(() => {
  const normalize = s => (s || '').replace(/\\s+/g, ' ').trim().toLowerCase();
  const TARGET = normalize(__NAME__);
  __SHADOW__
  const candidates = deepQuery('a, [role="link"], button, [role="button"], td, span, div');
  let best = null;
  for (const el of candidates) {
    if (el.children.length > 2) continue;
    if (normalize(el.textContent) !== TARGET) continue;
    const rect = el.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) continue;
    if (rect.top < 120) continue;
    const isLink = el.tagName === 'A' || el.getAttribute('role') === 'link';
    if (!best || (isLink && !best.isLink)) best = { el, isLink, top: rect.top };
  }
  if (!best) return false;
  const clickable = best.el.closest('a, [role="link"], button, [role="button"], tr, li') || best.el;
  clickable.click();
  return true;
})()
"""


async def _open_list_item(browser: "BrowserSession", name: str) -> bool:
    """Deterministically open a list row by its exact visible name. Returns True
    only if a row whose text exactly equals `name` was found and clicked."""
    name = (name or "").strip()
    if not name:
        return False
    js = (_OPEN_LIST_ITEM_JS_TEMPLATE
          .replace("__SHADOW__", _SHADOW_WALK_JS)
          .replace("__NAME__", json.dumps(name)))
    try:
        # Run across all frames — Azure list rows live in a cross-origin iframe.
        clicked = await _cdp_eval_in_frames(browser, js)
    except Exception:
        return False
    if not clicked:
        return False
    await asyncio.sleep(0.8)  # let the resource blade start loading
    return True


# Reads the EXACT resource names from an Azure list/grid, in code — no LLM. The
# discovery model corrupts and truncates long lists (drops characters, adds
# stray dots, cuts the JSON off), and those names then fail to open because
# open_list_item needs an exact match. Every resource Name in an Azure browse
# grid is an <a> whose href contains "/providers/<ns>/<type>/<name>" — the
# Resource Group and Subscription links in the same row do NOT ("/resourceGroups"
# and "/subscriptions" respectively), so that pattern cleanly isolates the Name
# column. Resource names never contain spaces, so we also skip any anchor text
# with whitespace (e.g. a "Partially protected" status link).
# __TYPE__ = "/providers/<ns>/<type>/" for the list currently being browsed
# (derived from the page URL), or "" if unknown. Azure is an SPA that keeps OLD
# blade iframes in the DOM after you navigate (e.g. Storage → Key Vaults), so an
# unscoped read mixes in stale names from a previous list. Two guards prevent
# that: (1) when the browsed type is known, only that type's Name links count;
# (2) skip anything not currently rendered/visible (display:none, checkVisibility
# false, slid off-screen) and any background tab — that's how Azure parks the
# previous blade.
_READ_LIST_NAMES_JS_TEMPLATE = """
(() => {
  __SHADOW__
  if (document.visibilityState && document.visibilityState !== 'visible') return null;
  const TYPE = __TYPE__;
  const W = window.innerWidth || 1280;
  const anchors = deepQuery('a');
  const names = [];
  const seen = {};
  for (const a of anchors) {
    const href = a.getAttribute('href') || '';
    if (TYPE) { if (href.indexOf(TYPE) === -1) continue; }
    else if (!/\\/providers\\/[^/]+\\/[^/]+\\/[^/?#]+/.test(href)) continue;
    const txt = (a.textContent || '').replace(/\\s+/g, ' ').trim();
    if (!txt || /\\s/.test(txt)) continue;   // resource names have no spaces
    const rect = a.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) continue;              // display:none etc.
    if (typeof a.checkVisibility === 'function' && !a.checkVisibility()) continue;
    if (rect.right <= 0 || rect.left >= W) continue;                // stale blade slid off-screen
    if (seen[txt]) continue;
    seen[txt] = 1;
    names.push(txt);
  }
  return names.length ? names : null;
})()
"""


def _current_browse_type_filter(top_url: str | None) -> str:
    """From the portal URL, return the "/providers/<ns>/<type>/" fragment of the
    list currently being browsed (e.g. Key Vaults → "/providers/Microsoft.KeyVault/vaults/"),
    or "" if the URL doesn't encode it (e.g. the Storage Center custom view). Used
    to keep the deterministic name read scoped to the CURRENT list's resource type,
    so leftover blades of other types can't leak their names in."""
    if not top_url:
        return ""
    m = re.search(r'#browse/([^/?#]+)', top_url)
    if not m:
        return ""
    seg = m.group(1).replace('%2F', '/').replace('%2f', '/')
    return f"/providers/{seg}/" if "/" in seg else ""


async def _read_azure_list_names(browser: "BrowserSession") -> list[str]:
    """Read the exact resource names visible in an Azure browse list, scrolling
    to pick up virtualized rows, and return them de-duplicated in order. Empty
    list if no resource-name links are found (caller falls back to the model)."""
    top_url = await _cdp_eval(browser, "window.location.href")
    type_filter = _current_browse_type_filter(top_url)
    js = (_READ_LIST_NAMES_JS_TEMPLATE
          .replace("__SHADOW__", _SHADOW_WALK_JS)
          .replace("__TYPE__", json.dumps(type_filter)))
    names: list[str] = []
    seen: set[str] = set()
    prev = -1
    for _ in range(40):  # generous cap; loop exits as soon as no new names appear
        try:
            batch = await _cdp_eval_in_frames(browser, js)
        except Exception:
            batch = None
        for n in batch or []:
            n = (n or "").strip()
            if n and n not in seen:
                seen.add(n)
                names.append(n)
        if len(names) == prev:
            break  # scrolling revealed nothing new → reached the end
        prev = len(names)
        try:
            await _wheel_scroll(browser, 900)
        except Exception:
            break
        await asyncio.sleep(0.4)
    return names


def _is_azure_portal(url: str | None) -> bool:
    """True only when the browser is on the Azure Portal — the gate that keeps
    all Azure-specific hardening from ever touching AWS/GitHub runs."""
    return bool(url) and "portal.azure.com" in url


# Registered only for Azure subtasks (see execute_task). Tells the LLM to reach
# for the deterministic tools instead of hand-clicking numbered elements, which
# is the whole point — the tools resolve their target fresh at click time and
# so are immune to Azure's re-rendering staleness.
AZURE_TOOL_INSTRUCTIONS = """\

AZURE PORTAL — USE THE DETERMINISTIC TOOLS (IMPORTANT):
You are on the Azure Portal, whose left-hand resource menu and lists re-render
constantly, which makes clicking a numbered element by index unreliable. For
these actions you MUST use the dedicated tools, not manual clicking/typing:
- To filter/narrow a LIST (e.g. the Key Vaults list): call fill_list_filter with
  the text. It types into the list's OWN "Filter for any field" box. NEVER type
  into the big global search bar at the very top ("Search resources, services,
  and docs") — that does not filter the list and will send you to the wrong place.
- To open a specific row from a list (e.g. a specific Key Vault by name): call
  open_list_item with the item's exact name. Do NOT click a numbered row — the
  rows look alike and you will open the wrong one.
- To open an item in the LEFT resource menu (e.g. Overview, Settings, Properties,
  Networking, Metrics, CLI / PS): call click_menu_item with the item's exact
  visible text. If the item lives inside a collapsible section (e.g. "Properties"
  under "Settings", or "CLI / PS" under "Automation"), also pass that section.
  This clicks the LEFT-NAV item, never a same-named tab in the main content area.
Only fall back to manual clicking if a tool reports it could not find the target.
"""


def _build_azure_tools(browser: "BrowserSession") -> "Tools":
    """Build a Tools registry exposing the two deterministic Azure helpers to
    the LLM. Only attached to the Agent when the current page is the Azure
    Portal, so AWS/GitHub runs keep the default toolset unchanged."""
    tools = Tools()

    @tools.action(
        "Click an item in the Azure Portal LEFT resource menu by its visible text "
        "(e.g. 'Properties', 'Networking', 'Metrics', 'CLI'). The start of the label is "
        "enough — 'CLI' matches 'CLI / PS'. If the item is nested inside a collapsible "
        "section, pass that section's name too (e.g. item='Properties', section='Settings'; "
        "or item='CLI', section='Automation'); the tool expands the section only if needed "
        "(it never collapses an already-open one). Clicks the left-nav item, never a "
        "same-named tab in the page body. ALWAYS use this for left-menu navigation — do NOT "
        "click left-menu items by numbered index, which mis-clicks and wastes steps."
    )
    async def click_menu_item(item: str, section: str = "", browser_session: BrowserSession = None) -> ActionResult:
        ok = await _click_resource_menu_item(browser_session or browser, item, section)
        where = f'"{item}"' + (f' under "{section}"' if section else "")
        if ok:
            return ActionResult(
                extracted_content=f"Clicked left-menu item {where} deterministically.",
                include_in_memory=True,
            )
        return ActionResult(
            extracted_content=(
                f"Could not confidently find left-menu item {where}. It may be spelled "
                f"differently, not nested where expected, or not present — look at the page "
                f"and try clicking it manually."
            ),
            include_in_memory=True,
        )

    @tools.action(
        "Type text into the Azure Portal's LOCAL list filter/search box (e.g. the Key "
        "Vaults list filter) and apply it — never the global search bar at the top of the "
        "page. Use this to narrow a list before reading it."
    )
    async def fill_list_filter(text: str, browser_session: BrowserSession = None) -> ActionResult:
        ok = await _deterministic_fill_filter(browser_session or browser, text)
        if ok:
            return ActionResult(
                extracted_content=f'Filtered the list by "{text}" deterministically.',
                include_in_memory=True,
            )
        return ActionResult(
            extracted_content=(
                f'Could not find the list\'s local filter box for "{text}". Do NOT type into '
                f'the global search bar at the very top of the page ("Search resources, '
                f'services, and docs") — that searches ALL resource types, not this list, and '
                f'gives the wrong results. Instead, click directly into the "Filter for any '
                f'field" box on the list itself and type there.'
            ),
            include_in_memory=True,
        )

    @tools.action(
        "Open a specific row from an Azure Portal list by its EXACT name (e.g. open the "
        "Key Vault named 'choreo-customer-prod' from the Key Vaults list). Clicks the row "
        "whose text exactly matches — use this instead of clicking a numbered row, because "
        "list rows look alike and clicking by index opens the wrong one."
    )
    async def open_list_item(name: str, browser_session: BrowserSession = None) -> ActionResult:
        ok = await _open_list_item(browser_session or browser, name)
        if ok:
            return ActionResult(
                extracted_content=f'Opened list item "{name}" deterministically.',
                include_in_memory=True,
            )
        return ActionResult(
            extracted_content=(
                f'Could not find a list row exactly named "{name}". It may not be in the '
                f"current (possibly unfiltered) list — filter the list first with "
                f"fill_list_filter, or check the exact spelling."
            ),
            include_in_memory=True,
        )

    return tools


def _screens_nearly_identical(a: bytes, b: bytes, threshold: float = 0.015) -> bool:
    """Cheap perceptual comparison: downscale both images to a small grayscale
    thumbnail and compare mean absolute pixel difference. Used to detect
    "scrolling had no visible effect" (bottom reached, or nothing to scroll)
    without knowing anything about the page's DOM structure."""
    from PIL import Image

    ia = Image.open(io.BytesIO(a)).convert("L").resize((64, 64))
    ib = Image.open(io.BytesIO(b)).convert("L").resize((64, 64))
    pa, pb = ia.tobytes(), ib.tobytes()
    diff = sum(abs(x - y) for x, y in zip(pa, pb)) / len(pa)
    return (diff / 255) < threshold


async def _capture_scrolling_screenshots(browser: "BrowserSession", max_shots: int = 60) -> list[Path]:
    """Scroll top-to-bottom using real mouse-wheel events and capture one
    screenshot per stop, covering the FULL page however long it is.

    Each step: dispatch a wheel event roughly one viewport tall, wait for
    render, then compare a cheap viewport-only probe screenshot against the
    previous one. If they're visually near-identical, scrolling had no
    effect (bottom reached, or the page/panel doesn't scroll) and capture
    stops — this replaces trying to compute scroll distance/position from
    the DOM, which can't see into iframes or virtualized lists.

    `max_shots` is a safety ceiling against runaway pages, not a normal-case
    limit — a real page should never get close to it.

    Each kept position is saved as a separate PNG with an evidence header
    stamped on top (URL, title, timestamp, user email). Returns one Path per
    scroll position.
    """
    # Brief settle delay before capturing anything — the agent may have just
    # finished a pagination click or navigation and called done() immediately
    # per its instructions, before the console's client-side render finished.
    await asyncio.sleep(0.6)

    url = (await _cdp_eval(browser, "window.location.href")) or ""
    title = (await _cdp_eval(browser, "document.title")) or ""
    viewport_h = await _cdp_eval(browser, "window.innerHeight") or 800

    async def _probe() -> bytes:
        """Fast, viewport-scoped screenshot used only to detect whether
        scrolling changed anything — not saved as evidence."""
        return await browser.take_screenshot(full_page=False)

    async def _evidence_snap() -> bytes:
        # Try real OS window capture first (shows the OS clock/chrome)
        try:
            return _capture_os_screenshot()
        except Exception:
            pass
        # Fall back: CDP screenshot with evidence header stamped on top
        raw = await browser.take_screenshot(full_page=False)
        try:
            return _add_evidence_header(raw, url, title)
        except Exception:
            return raw

    # Best-effort reset to the top (harmless if already there)
    for _ in range(6):
        await _wheel_scroll(browser, -viewport_h * 3)
    await asyncio.sleep(0.3)

    async def _snap_here() -> None:
        data = await _evidence_snap()
        pth = _SCREENSHOTS_DIR / f"{uuid.uuid4()}.png"
        pth.write_bytes(data)
        paths.append(pth)

    paths: list[Path] = []
    probes: list[bytes] = []

    # First frame — top of the page.
    await _snap_here()
    probes.append(await _probe())

    # Cover the WHOLE page: scroll ~0.9 viewport each step (≈10% overlap, so no
    # strip is ever skipped between shots) and ALWAYS take a screenshot at the new
    # position — we NEVER skip a capture, so no part of the page can be missed
    # even if a frame happens to look similar to the previous one. The "looks the
    # same" check only decides when to STOP: after TWO no-move scrolls in a row we
    # conclude we've reached the bottom (a single one can be a wheel event that
    # landed on a fixed/non-scrolling region and moved nothing). The couple of
    # duplicate frames captured at the very bottom are trimmed afterward.
    consecutive_same = 0
    for _ in range(max_shots - 1):
        await _wheel_scroll(browser, viewport_h * 0.9)
        await asyncio.sleep(0.5)
        probe = await _probe()
        await _snap_here()  # capture BEFORE deciding anything — never skip a strip
        if _screens_nearly_identical(probes[-1], probe):
            consecutive_same += 1
            probes.append(probe)
            if consecutive_same >= 2:
                break  # two scrolls in a row moved nothing → bottom reached
            continue
        consecutive_same = 0
        probes.append(probe)

    # Drop the trailing duplicate frames captured at the bottom (where scrolling
    # no longer moved the page), so evidence isn't padded with identical shots —
    # only exact trailing duplicates, never a real content frame.
    while len(paths) > 1 and _screens_nearly_identical(probes[-1], probes[-2]):
        dup = paths.pop()
        probes.pop()
        try:
            dup.unlink()
        except Exception:
            pass

    return paths


async def _capture_evidence_screenshots(browser: "BrowserSession", history) -> list[Path]:
    """Capture all scroll positions as individual screenshots for compliance evidence.
    Falls back to a single viewport screenshot if scrolling fails."""
    try:
        paths = await _capture_scrolling_screenshots(browser)
        if paths:
            return paths
    except Exception:
        pass

    try:
        p = _SCREENSHOTS_DIR / f"{uuid.uuid4()}.png"
        await browser.take_screenshot(path=str(p), full_page=False)
        return [p]
    except Exception:
        screenshots = history.screenshots()
        if screenshots:
            return [_save_screenshot_local(screenshots[-1])]
        return []


# ── PDF capture (for "PDF:" subtasks, e.g. exporting a GitHub issue) ───────

_PDF_DIR = Path.home() / ".wso2-runner" / "pdfs"
_PDF_DIR.mkdir(parents=True, exist_ok=True)

# Prepended to the agent's task text for "PDF:" subtasks. Getting the FULL
# thread expanded before the PDF is rendered is the whole point — GitHub
# issues collapse long comment threads behind "Load more"/"Show more
# comments" buttons, sometimes more than once on very long issues.
PDF_EXPAND_INSTRUCTIONS = """\
THIS IS A STRICT READ-ONLY CAPTURE TASK. Follow this exact procedure —
nothing else:

  1. Navigate to the URL given in TASK below.
  2. Look at the page for a button whose text is exactly (or very close to)
     "Load more", "Show more comments", or "Load more comments".
  3. If one is visible, click ONLY that button. Wait for it to load.
  4. Repeat steps 2-3 until no such button remains anywhere on the page.
  5. Call done().

That is the ENTIRE task. In the common case there is no such button at all,
which means step 5 happens immediately after step 1 — that is correct and
expected, not a sign you should keep looking for something to do.

ABSOLUTELY DO NOT, under any circumstances, click, focus, hover-select, or
otherwise interact with any of the following, even briefly or "just to
check" — none of them help this task and all of them are explicitly
forbidden: Labels, Projects, Milestone, Assignees, Relationships, any gear/
settings icon, "New issue", "Create sub-issue", "Add existing issue",
"Draft with Copilot", "Comment", "Close issue", "Close with comment", any
Edit (pencil) icon, any filter/search/sort control, any tab other than the
one you landed on, the "Add a comment" text box, or the site's own global
navigation menu/logo in the top-left corner (the one with Projects,
Discussions, Codespaces, Copilot, Explore, Marketplace, etc.) — you never
need to navigate anywhere else; you are already on the correct page. If you
do not recognize a button as literally being a "Load more" / "Show more comments" control,
do NOT click it — skip it and move on to done().

Do not type anything, anywhere, at any point in this task.

If you notice a dropdown, menu, popup, or focused input box is open (for
any reason, including by accident), press Escape and/or click a neutral
blank area to close/clear it before calling done() — the exported capture
must show only the plain, at-rest page.

Do not worry about scrolling or taking a screenshot yourself — the system
captures the whole page as a PDF automatically the moment you call done().

TASK:
"""


_EXPAND_LOAD_MORE_JS = """
(() => {
  const patterns = ['load more', 'show more', 'hidden item', 'more comments', 'collapsed'];
  // Only ever consider genuinely clickable elements — never a generic div/span.
  // querySelectorAll returns document order, so a wrapping <div> that contains
  // BOTH a "54 remaining items" heading AND a nested "Load more" <button> as
  // children would be found before that button, and its own combined text
  // ("54 remaining items load more") also matches — clicking that div does
  // nothing, since it isn't the real interactive element. Restricting to
  // button/a/summary/[role=button] guarantees we click something with an
  // actual click handler, not a non-interactive ancestor container.
  const candidates = document.querySelectorAll('button, a, summary, [role="button"]');
  for (const el of candidates) {
    const text = (el.textContent || '').trim().toLowerCase();
    if (!text || text.length > 40) continue;
    if (patterns.some(p => text.includes(p))) {
      el.click();
      return true;
    }
  }
  return false;
})()
"""


async def _expand_load_more(browser: "BrowserSession", max_iterations: int = 20) -> None:
    """Deterministically find and click any "Load more" / "Show more" / "N
    hidden items" style element anywhere on the page, repeatedly, before
    capture — regardless of what the agent's own reasoning noticed or
    scrolled past.

    Why this needs to be code, not just a prompt instruction: GitHub
    collapses a chunk of comments in the MIDDLE of a long thread (between
    the first few and last few), not only at the bottom. An agent that
    doesn't methodically scroll the entire page before deciding "looks
    complete" can miss that divider entirely, baking the unexpanded
    placeholder into the PDF. A plain JS `.click()` on the matched element
    works regardless of scroll position — no need to scroll it into view
    first — and re-scanning after each click catches sections that reveal
    another collapsed section further down.
    """
    for _ in range(max_iterations):
        try:
            clicked = await _cdp_eval(browser, _EXPAND_LOAD_MORE_JS)
        except Exception:
            break
        if not clicked:
            break
        await asyncio.sleep(0.5)  # let newly-revealed content render before re-scanning


async def _capture_pdf(browser: "BrowserSession") -> Path:
    """Render the current page to a PDF via CDP Page.printToPDF.

    Deliberately headless/protocol-level rather than driving the OS/browser
    print dialog: that dialog is native OS chrome outside the page the agent
    can see or interact with reliably, and its layout/shortcuts differ per
    platform. printToPDF gives the same result on any machine with no
    dialog automation involved.

    Before capturing: (1) deterministically expand any remaining "Load more"
    sections (see _expand_load_more — covers what the agent's own scan may
    have missed), then (2) force-close any dropdown/menu/popup the agent may
    have left open (GitHub's nav menu, a sub-issue picker, etc.) regardless
    of whether it followed the "don't touch anything" instructions — Escape
    is the universal close-overlay convention on the web. Neither step
    depends on the LLM's behavior at all.
    """
    try:
        await _expand_load_more(browser)
    except Exception:
        pass

    try:
        page = await browser.must_get_current_page()
        await page.press("Escape")
        await asyncio.sleep(0.2)
        await page.press("Escape")
        await asyncio.sleep(0.2)
    except Exception:
        pass

    cdp_session = await browser.get_or_create_cdp_session()
    result = await cdp_session.cdp_client.send.Page.printToPDF(
        params={"printBackground": True},
        session_id=cdp_session.session_id,
    )
    pdf_bytes = base64.b64decode(result["data"])
    path = _PDF_DIR / f"{uuid.uuid4()}.pdf"
    path.write_bytes(pdf_bytes)
    return path


async def _capture_evidence_pdf(browser: "BrowserSession") -> list[Path]:
    """Capture the current page as a single PDF for compliance evidence.
    Falls back to a regular scrolling-screenshot capture if PDF export fails,
    so a subtask never produces zero evidence."""
    try:
        return [await _capture_pdf(browser)]
    except Exception as exc:
        print(f"[runner]   PDF export failed, falling back to screenshots: {exc}")
        try:
            return await _capture_scrolling_screenshots(browser)
        except Exception:
            return []


# ── Browser singleton ──────────────────────────────────────────────────────

_browser: BrowserSession | None = None


def _get_browser() -> BrowserSession:
    global _browser
    if _browser is None:
        channel = (settings.BROWSER_CHANNEL or "chrome").strip().lower()
        profile_dir = _PROFILE_DIR / channel
        profile_dir.mkdir(exist_ok=True)
        profile = BrowserProfile(
            channel=channel,
            headless=False,
            user_data_dir=str(profile_dir),
            keep_alive=True,
        )
        _browser = BrowserSession(browser_profile=profile)
    return _browser


async def reset_browser() -> dict:
    """Kill the cached BrowserSession so the next login starts a fresh window
    (recovery path for a stuck/crashed browser).

    Invoked for Agent Task kind == "reset", which is emitted by the web app's
    reset-session control (arriving in the web app PR) — this handler ships
    ahead of its emitter."""
    global _browser
    if _browser is not None:
        try:
            await _browser.kill()
        except Exception:
            pass
        try:
            await _browser.stop()
        except Exception:
            pass
    _browser = None
    return {"status": "completed", "result": "Browser session reset.", "screenshots": []}


async def open_login_browser(url: str) -> dict:
    """Open the same persistent browser/profile used for agent tasks and
    navigate to `url`, so the user can manually authenticate (incl. MFA).
    Because the profile is persistent and the session is kept alive, the
    cookies set here are reused by execute_task() for subsequent tasks."""
    browser = _get_browser()
    await browser.start()
    await browser.navigate_to(url)
    return {
        "status": "completed",
        "result": (
            f"Browser opened at {url}. Complete login manually (including MFA) "
            "in that browser window, then queue your real task — it will reuse "
            "this same authenticated session."
        ),
        "screenshots": [],
    }


# ── LLM factory ───────────────────────────────────────────────────────────

def _build_llm():
    provider = settings.AGENT_PROVIDER
    model = settings.AGENT_MODEL

    if provider == "anthropic":
        # browser-use's own ChatAnthropic (not langchain_anthropic's) — the
        # Agent class requires its LLM objects to implement browser-use's own
        # interface (e.g. a `.provider` property); a raw langchain_anthropic
        # ChatAnthropic doesn't have that and fails with
        # "'ChatAnthropic' object has no attribute 'provider'" as soon as an
        # Agent is constructed with it.
        from browser_use import ChatAnthropic
        kwargs = {"model": model or "claude-haiku-4-5-20251001", "api_key": settings.ANTHROPIC_API_KEY}
        if settings.ANTHROPIC_BASE_URL:
            kwargs["base_url"] = settings.ANTHROPIC_BASE_URL
        return ChatAnthropic(**kwargs)
    elif provider == "gemini":
        from langchain_google_genai import ChatGoogleGenerativeAI
        return ChatGoogleGenerativeAI(model=model or "gemini-2.0-flash", google_api_key=settings.GEMINI_API_KEY)
    elif provider == "azure":
        from browser_use import ChatAzureOpenAI
        return ChatAzureOpenAI(
            model=model or "gpt-4o-mini",
            api_key=settings.AZURE_OPENAI_API_KEY,
            azure_endpoint=settings.AZURE_OPENAI_ENDPOINT,
            azure_deployment=settings.AZURE_OPENAI_DEPLOYMENT,
            api_version=settings.AZURE_OPENAI_API_VERSION,
        )
    else:
        from browser_use import ChatOllama
        return ChatOllama(model=model or "qwen2.5:7b", timeout=180)


def _base_url_override_note(base_url: str) -> str:
    """Overrides the generic (and possibly stale) TARGET PORTAL note from
    _build_context_prefix() for subtasks that carry a known-correct base_url
    (EACH:/EACH-PAGE: discovery + their expanded subtasks).

    TARGET PORTAL is set once, at Step 1 login, and never changes for the
    rest of the run — e.g. if the user logged in via their own GitHub
    profile URL just to authenticate, every subtask still carries "stay
    within https://github.com/<their-username>" even after a later subtask
    has correctly navigated to a completely different repo. A fresh Agent()
    instance (discovery, or any expanded per-page/per-item subtask) has no
    other memory of where it actually is, so it can end up treating that
    stale login URL as instruction to "go back home" instead of trusting
    the browser's actual current location — this is exactly what caused the
    agent to land on the wrong repo/account. This note takes priority.
    """
    return (
        f"\nIMPORTANT LOCATION OVERRIDE: you are ALREADY on the correct page for THIS task: "
        f"{base_url}\nThis is more specific and takes priority over any general portal/login URL "
        f"noted above — do NOT navigate to that general URL, and do NOT navigate anywhere else "
        f"at all unless this task's own instructions explicitly tell you to click something. "
        f"Stay on this exact page.\n"
    )


def _build_context_prefix(region_hint: str | None, portal_url: str | None = None) -> str:
    parts: list[str] = []
    if portal_url and portal_url.strip():
        parts.append(
            f"TARGET PORTAL: {portal_url.strip()}\n"
            "You are already on this portal (the user logged in before starting this task). "
            "Stay within this portal unless the task explicitly requires navigating elsewhere. "
            "Do NOT navigate to unrelated websites (e.g. Amazon, Google, Wikipedia).\n"
        )
    if region_hint and region_hint.strip():
        parts.append(
            "ENVIRONMENT CONTEXT (apply to all tasks):\n"
            f"{region_hint.strip()}\n"
            "Before searching for any resource, switch to the correct region/subscription/"
            "workspace mentioned above — but ONLY for services that are actually region-scoped "
            "in their console view (e.g. EC2 instances, RDS, VPC — where selecting a different "
            "region genuinely hides resources from other regions).\n"
            "Some services list resources account-wide in a single flat view regardless of the "
            "selected region, showing each resource's region as an informational column only "
            "(e.g. S3 buckets, IAM users/roles, Route 53). For these, do NOT exclude or ignore a "
            "resource just because its displayed region differs from the hint above — that column "
            "is informational, not a filter, and the resource is still the one being asked for.\n"
        )
    return "\n".join(parts) + "\n" if parts else ""


# ── Main execution function ────────────────────────────────────────────────

async def execute_task(
    task: dict,
    on_subtask_done: Callable[[list, int, list[Path], dict], Awaitable[None]],
    on_pause: Callable[[list, int, dict], Awaitable[None]] | None = None,
) -> dict:
    """
    Run one agent task end-to-end.

    Args:
        task: TaskOut dict from cloud backend
        on_subtask_done(subtask_states, idx, local_screenshot_path, total_usage):
            Called after each subtask. Loop uses this to upload screenshot + post progress.
        on_pause(subtask_states, idx, total_usage): optional. Called after a step
            carrying a {PAUSE} marker; should block until the user clicks Resume
            (the loop implements it by polling the backend).

    Returns:
        Result dict to pass to client.post_result()
    """
    llm = _build_llm()
    browser = _get_browser()
    await browser.start()

    subtask_specs = parse_subtasks(task["prompt"])
    max_steps = int(task.get("max_steps") or 25)
    use_vision = bool(task.get("use_vision")) if task.get("use_vision") is not None else False
    max_actions_per_step = int(task.get("max_actions_per_step") or 1)
    context_prefix = _build_context_prefix(task.get("region_hint"), task.get("portal_url"))

    subtask_states: list[dict] = [
        {"index": i, "text": s["text"], "kind": s["kind"], "pause": s.get("pause", False),
         "status": "pending", "result": None, "screenshots": [], "usage": None}
        for i, s in enumerate(subtask_specs)
    ]
    total_usage: dict = {
        "input_tokens": 0, "output_tokens": 0, "total_tokens": 0,
        "llm_calls": 0, "cost_usd": 0.0, "model": settings.AGENT_MODEL,
        "provider": settings.AGENT_PROVIDER,
    }
    all_results: list[str] = []

    counter = _attach_token_counter(llm)

    # Manual index (not `for idx, x in enumerate(...)`) because template
    # subtasks splice extra literal subtasks into subtask_states in place —
    # a plain for-loop's iterator would skip the first spliced-in item since
    # its internal position already advanced past the current slot.
    idx = 0
    while idx < len(subtask_states):
        subtask_obj = subtask_states[idx]

        if subtask_obj.get("kind") in ("each", "each_page"):
            subtask_obj["status"] = "running"
            subtask_obj["started_at"] = time.time()
            await on_subtask_done(subtask_states, idx, None, total_usage)  # shows "discovering..."

            # Capture wherever the previous subtask left the browser — this is
            # the URL every subsequent step (discovery + each expanded
            # subtask) gets forced back to in code, so a drifted agent can
            # never "recover" by searching its own account for a similarly
            # named resource instead of the literal target.
            base_url = await _cdp_eval(browser, "window.location.href")

            try:
                expanded_texts = await _expand_template_subtask(
                    subtask_obj["kind"], subtask_obj["text"], llm, browser, context_prefix, max_steps,
                    base_url=base_url,
                )
            except Exception as exc:
                expanded_texts = []
                subtask_obj["status"] = "completed"
                subtask_obj["result"] = f"Discovery failed: {exc}"
                subtask_obj["completed_at"] = time.time()
                await on_subtask_done(subtask_states, idx, None, total_usage)
                idx += 1
                continue

            if not expanded_texts:
                subtask_obj["status"] = "completed"
                subtask_obj["result"] = "Discovery found no matching items."
                subtask_obj["completed_at"] = time.time()
                await on_subtask_done(subtask_states, idx, None, total_usage)
                idx += 1
                continue

            new_entries = [
                {"index": 0, "text": t, "kind": "literal", "status": "pending", "result": None, "screenshots": [],
                 "usage": None, "base_url": base_url}
                for t in expanded_texts
            ]
            subtask_states[idx:idx + 1] = new_entries
            for i, s in enumerate(subtask_states):
                s["index"] = i
            await on_subtask_done(subtask_states, idx, None, total_usage)  # notify: list expanded
            continue  # re-process at the same idx — now the first expanded item

        if subtask_obj.get("kind") == "filter":
            subtask_obj["status"] = "running"
            subtask_obj["started_at"] = time.time()
            await on_subtask_done(subtask_states, idx, None, total_usage)

            # Capture the current page before attempting the fill — if we end
            # up falling back to an agent-driven attempt below, this anchors
            # it to "stay here" via the same base_url mechanism EACH:/
            # EACH-PAGE: already use, instead of leaving a fresh agent with
            # zero anchoring to wander off (e.g. back to Home / All resources).
            filter_page_url = await _cdp_eval(browser, "window.location.href")

            filled = await _deterministic_fill_filter(browser, subtask_obj["text"])
            if filled:
                subtask_obj["status"] = "completed"
                subtask_obj["result"] = (
                    f'Filter box filled deterministically with "{subtask_obj["text"]}" (no LLM call needed).'
                )
                subtask_obj["completed_at"] = time.time()
                await on_subtask_done(subtask_states, idx, None, total_usage)
                idx += 1
                continue
            # No confident target found — fall back to normal agent-driven
            # typing instead of silently doing nothing. Convert in place to
            # an ordinary literal subtask with explicit disambiguation
            # instructions AND a base_url anchor, then fall through to the
            # standard execution path below (no `continue` here on purpose).
            subtask_obj["kind"] = "literal"
            subtask_obj["base_url"] = filter_page_url
            subtask_obj["text"] = (
                f'Find the LOCAL filter/search box that is part of THIS list/page itself — '
                f'NOT the global search bar at the very top of the page. Type '
                f'"{subtask_obj["text"]}" into it, press Enter, and confirm the results have '
                f'visibly filtered before finishing.'
            )

        subtask_obj["status"] = "running"
        subtask_obj["started_at"] = time.time()
        # Notify: this subtask is now running (no screenshot yet)
        await on_subtask_done(subtask_states, idx, None, total_usage)

        # Subtasks expanded from EACH:/EACH-PAGE: carry the URL the browser
        # was on before expansion started — force back to it deterministically
        # rather than trusting the agent stayed put or can find its own way
        # back if it drifted (see _expand_template_subtask docstring).
        # Only navigate if the browser has actually drifted from that URL —
        # a hard re-navigation would otherwise wipe client-side-only state
        # (e.g. a search/filter box whose value never appears in the URL,
        # like the AWS S3 console's bucket name filter) even when nothing
        # went wrong.
        subtask_base_url = subtask_obj.get("base_url")
        subtask_location_note = ""
        if subtask_base_url:
            try:
                current_url = await _cdp_eval(browser, "window.location.href")
                if current_url != subtask_base_url:
                    await browser.navigate_to(subtask_base_url)
            except Exception:
                pass
            subtask_location_note = _base_url_override_note(subtask_base_url)

        snap = (counter.input_tokens, counter.output_tokens, counter.calls)
        is_pdf = subtask_obj.get("kind") == "pdf"
        extra_instructions = PDF_EXPAND_INSTRUCTIONS if is_pdf else ""

        # Azure Portal hardening — gated strictly on the current URL so AWS and
        # GitHub runs are byte-for-byte unchanged (no extra tools, no extra
        # instructions, no extra wait). On Azure we (1) hand the agent the two
        # deterministic helpers (click_menu_item / fill_list_filter) that resolve
        # their target fresh at click time and so survive Azure's constant
        # re-rendering, and (2) let the blade settle before the run starts.
        current_page_url = await _cdp_eval(browser, "window.location.href")
        azure_tools = None
        azure_instructions = ""
        if _is_azure_portal(current_page_url):
            azure_tools = _build_azure_tools(browser)
            azure_instructions = AZURE_TOOL_INSTRUCTIONS
            await asyncio.sleep(1.0)  # let the Ibiza blade finish rendering

        full_task = (
            AGENT_INSTRUCTIONS + extra_instructions + azure_instructions
            + context_prefix + subtask_location_note + subtask_obj["text"]
        )
        agent_kwargs = dict(
            task=full_task, llm=llm, browser=browser,
            use_vision=use_vision, max_actions_per_step=max_actions_per_step,
        )
        if azure_tools is not None:
            agent_kwargs["tools"] = azure_tools
        agent = Agent(**agent_kwargs)
        history = await agent.run(max_steps=max_steps)

        in_t = counter.input_tokens - snap[0]
        out_t = counter.output_tokens - snap[1]
        calls = counter.calls - snap[2]
        cost = _compute_cost(in_t, out_t, settings.AGENT_MODEL)

        result_text = str(history.final_result() or f"Subtask {idx + 1} completed")
        all_results.append(result_text)

        subtask_obj["result"] = result_text
        subtask_obj["status"] = "completed"
        subtask_obj["completed_at"] = time.time()
        subtask_obj["usage"] = {
            "input_tokens": in_t, "output_tokens": out_t,
            "total_tokens": in_t + out_t, "llm_calls": calls,
            "cost_usd": cost, "model": settings.AGENT_MODEL,
        }

        total_usage["input_tokens"] += in_t
        total_usage["output_tokens"] += out_t
        total_usage["total_tokens"] += in_t + out_t
        total_usage["llm_calls"] += calls
        total_usage["cost_usd"] = round(total_usage["cost_usd"] + cost, 6)

        # PDF: subtasks export the whole (expanded) page as one PDF; everything
        # else captures one screenshot per scroll position, same as before.
        if is_pdf:
            local_paths = await _capture_evidence_pdf(browser)
        else:
            local_paths = await _capture_evidence_screenshots(browser, history)

        # Notify: subtask done. Loop will upload all screenshots + post progress.
        await on_subtask_done(subtask_states, idx, local_paths, total_usage)

        # {PAUSE}: after this step's screenshot, hand control to the user (they
        # set up filters manually) until they click Resume, then carry on. The
        # next step (e.g. an EACH read) then sees their filtered list.
        if subtask_obj.get("pause") and on_pause is not None:
            await on_pause(subtask_states, idx, total_usage)

        idx += 1

    combined_result = "\n\n".join(
        f"[Task {i + 1}] {r}" for i, r in enumerate(all_results)
    )

    total_usage["subtask_count"] = len(subtask_states)

    return {
        "status": "completed",
        "result": combined_result,
        "screenshots": [s for sub in subtask_states for s in sub.get("screenshots", [])],
        "total_usage": total_usage,
    }
