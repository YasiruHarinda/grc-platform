"""Builds the readable `product/framework/control/` blob-name prefix and the
sanitised evidence-title label that go into `save_file`'s `prefix`/`label`
arguments (see `app.storage.blob_storage.save_file`).

Kept out of `blob_storage.py` deliberately: that module talks to the Azure
SDK and knows nothing about this app's domain models, and it should stay
that way -- it's reusable for any blob, hierarchical name or not. This
module is the domain-aware half: it knows a Control sits under a Framework
under a Product, and turns that chain into a blob-name prefix. Nothing here
touches storage or the database; it only turns already-loaded ORM objects
and strings into path/name fragments.

Path shape (see fork issue #70's addendum): three NAME-based segments, no
`evidence` level --

    product/framework/control/{sanitised-title}-{uuid}{ext}

Segment/title names are sanitised the same way (`_sanitize` below):
lowercased, whitespace/underscores collapsed to single hyphens, anything
outside `[a-z0-9-]` stripped outright (this is what guarantees a `/` in a
Product/Framework/Control name or an Evidence title can never survive into
a path segment -- it's simply not in the allowed character set), repeated
hyphens collapsed, and leading/trailing hyphens trimmed. A value that
sanitises to nothing (blank, punctuation-only, or entirely non-ASCII, e.g.
some unicode titles) falls back to a fixed, always-non-empty label instead
of producing a name like `-{uuid}.png`.

Length caps, chosen to keep the total blob name comfortably inside Azure's
1024-character blob-name limit even in a pathological worst case:

- `SEGMENT_MAX_LENGTH = 60` per Product/Framework/Control segment. These
  names are short, curated, admin-entered labels in practice (Product.name
  and Framework.name are capped at 100/50 chars by their own column types,
  Control.title at 255), so 60 is generous headroom without being unbounded.
- `TITLE_MAX_LENGTH = 120` for the sanitised Evidence-title label. Titles
  are free text -- a user-typed Evidence title, or an AI agent's task title
  / prompt -- so this is more generous than a segment cap, while still
  bounding it well short of a problem.

Worst case: 3 segments x (60 chars + 1 "/") + a 120-char title + "-" + a
36-char uuid4 + a several-char extension = 183 + 120 + 1 + 36 + ~5 = ~345
characters, well under the 1024-char limit with wide margin for the
extension or any miscount.
"""
import re
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from app.models.control import Control

# Per-segment and per-title caps -- see the module docstring for why these
# particular numbers were chosen.
SEGMENT_MAX_LENGTH = 60
TITLE_MAX_LENGTH = 120

# Used when a Product/Framework/Control name sanitises to nothing (blank,
# punctuation-only, or entirely non-ASCII) -- never produce an empty path
# segment.
FALLBACK_SEGMENT_LABEL = "unnamed"

# Used when an Evidence title sanitises to nothing -- never produce a blob
# name like `-{uuid}.png` with nothing in front of the hyphen.
FALLBACK_TITLE_LABEL = "evidence"

_WHITESPACE_OR_UNDERSCORE = re.compile(r"[\s_]+")
_UNSAFE_CHARS = re.compile(r"[^a-z0-9-]+")
_REPEATED_HYPHENS = re.compile(r"-{2,}")


def _sanitize(value: str | None, *, max_length: int, fallback: str) -> str:
    """Lowercase; collapse whitespace/underscores to single hyphens; strip
    anything outside `[a-z0-9-]` (this is what a `/`, or any unicode
    character, never survives); collapse repeated hyphens; trim leading/
    trailing hyphens; truncate to `max_length` (trimming again in case the
    cut lands on a trailing hyphen); fall back to `fallback` if nothing
    survives.
    """
    lowered = (value or "").lower()
    hyphenated = _WHITESPACE_OR_UNDERSCORE.sub("-", lowered)
    stripped = _UNSAFE_CHARS.sub("", hyphenated)
    collapsed = _REPEATED_HYPHENS.sub("-", stripped)
    trimmed = collapsed.strip("-")
    truncated = trimmed[:max_length].strip("-")
    return truncated or fallback


def sanitize_segment(value: str | None) -> str:
    """Sanitise a single Product/Framework/Control name into a safe,
    non-empty path segment."""
    return _sanitize(value, max_length=SEGMENT_MAX_LENGTH, fallback=FALLBACK_SEGMENT_LABEL)


def sanitize_title(value: str | None) -> str:
    """Sanitise (and truncate) an Evidence title into a safe, non-empty
    filename label."""
    return _sanitize(value, max_length=TITLE_MAX_LENGTH, fallback=FALLBACK_TITLE_LABEL)


def build_control_prefix(control: "Control") -> str:
    """The `product/framework/control/` blob-name prefix for evidence filed
    against `control`, walking `control.framework.product` (both
    relationships already exist -- no joins to write).

    Trailing slash included, so the result can be passed straight through
    as `save_file`'s `prefix`: `f"{prefix}{label}-{uuid}{ext}"`.
    """
    framework = control.framework
    product = framework.product
    segments = (
        sanitize_segment(product.name),
        sanitize_segment(framework.name),
        sanitize_segment(control.title),
    )
    return "/".join(segments) + "/"
