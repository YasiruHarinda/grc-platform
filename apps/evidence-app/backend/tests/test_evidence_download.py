"""
Coverage for `GET /api/evidence/{evidence_id}/download` (spec issue #92): a
zip of one Evidence's files, ordered, named and folder-nested so a reviewer
can extract several of these and get a correct
`product/framework/control/evidence-title/` audit-pack hierarchy back --
unlike clicking each signed-link screenshot individually, which loses the
order, the Control hierarchy and the readable name (all lost once the
browser is left holding only the blob's own UUID basename).

Authorization is reused, not reinvented: the route calls the exact same
`_authorize_evidence_access` (`app/api/routes/evidence.py:16`) that
`get_evidence` and `delete_evidence` already use, so it is covered here only
to the extent of confirming it is actually wired up and that it runs *before*
any blob is read -- not to re-litigate the rule itself (see
test_evidence_ownership.py / test_evidence_deletion_authorization.py for
that).

See ADR 0003 and the spec's "Relationship to ADR 0003" section for why this
authenticated, per-Evidence bulk download does not reopen the question that
removed the old unauthenticated `/uploads/{filename}` proxy route.
"""
import io
import zipfile

import azure.storage.blob

from app.models.evidence import Evidence
from app.models.evidence_file import EvidenceFile
from app.storage.blob_paths import FALLBACK_TITLE_LABEL

from tests.conftest import build_evidence, make_control, upload_blob

OWNER = "owner-download@example.com"


def _open_zip(response) -> zipfile.ZipFile:
    return zipfile.ZipFile(io.BytesIO(response.content))


# --- Happy path ------------------------------------------------------------


def test_authorized_owner_gets_200_and_a_valid_zip(db_session, engineer_client, engineer_user):
    evidence, _files = build_evidence(
        db_session, ("shot.png", b"valid zip bytes"), created_by=engineer_user.email
    )

    response = engineer_client.get(f"/api/evidence/{evidence.id}/download")

    assert response.status_code == 200
    assert response.headers["content-type"] == "application/zip"
    archive = _open_zip(response)
    assert archive.testzip() is None  # every member's CRC checks out
    assert len(archive.namelist()) == 1


def test_admin_can_download_evidence_they_do_not_own(db_session, admin_client):
    evidence, _files = build_evidence(db_session, ("shot.png", b"admin bytes"), created_by=OWNER)

    response = admin_client.get(f"/api/evidence/{evidence.id}/download")

    assert response.status_code == 200
    assert _open_zip(response).testzip() is None


# --- Authorization -----------------------------------------------------


def test_non_owner_engineer_gets_403_and_no_bytes_are_read_from_storage(
    db_session, engineer_client, monkeypatch
):
    evidence, _files = build_evidence(db_session, ("only.png", b"only bytes"), created_by=OWNER)

    def _fail_if_called(self, *args, **kwargs):
        raise AssertionError(
            f"download_blob() called for {self.blob_name!r}; the authorization "
            "check must run, and refuse, before any blob is read"
        )

    monkeypatch.setattr(azure.storage.blob.BlobClient, "download_blob", _fail_if_called)

    response = engineer_client.get(f"/api/evidence/{evidence.id}/download")

    # Same status _authorize_evidence_access already produces for a
    # non-owner Engineer (test_evidence_deletion_authorization.py) --
    # nothing here invents a new rule.
    assert response.status_code == 403


def test_unknown_evidence_id_returns_404(engineer_client):
    response = engineer_client.get("/api/evidence/999999/download")

    assert response.status_code == 404


def test_evidence_with_no_files_returns_404_not_an_empty_zip(
    db_session, engineer_client, engineer_user
):
    # Not reachable through any current upload path (both `create_evidence`
    # and the agent-result path always attach at least one EvidenceFile),
    # but the route must still refuse gracefully if it ever happens rather
    # than handing back a zip with nothing in it.
    evidence = Evidence(
        title="No files",
        file_name="orphaned.png",
        file_url="/uploads/orphaned.png",
        created_by=engineer_user.email,
    )
    db_session.add(evidence)
    db_session.commit()
    db_session.refresh(evidence)

    response = engineer_client.get(f"/api/evidence/{evidence.id}/download")

    assert response.status_code == 404


# --- Zip layout ----------------------------------------------------------


def test_zip_layout_with_a_control_nests_under_product_framework_control_title(
    db_session, engineer_client
):
    control = make_control(db_session)  # Test Product / Test Framework / Test Control
    evidence, _files = build_evidence(
        db_session, ("shot.png", b"control bytes"), control_id=control.id
    )

    response = engineer_client.get(f"/api/evidence/{evidence.id}/download")

    assert response.status_code == 200
    [entry] = _open_zip(response).namelist()
    assert entry.startswith(
        "test-product/test-framework/test-control/console-screenshot/01-console-screenshot-"
    ), entry
    assert entry.endswith(".png")


def test_zip_layout_without_a_control_sits_at_the_zip_root_with_no_wrapper(
    db_session, engineer_client
):
    evidence, _files = build_evidence(db_session, ("shot.png", b"no control bytes"))

    response = engineer_client.get(f"/api/evidence/{evidence.id}/download")

    assert response.status_code == 200
    [entry] = _open_zip(response).namelist()
    # Exactly one "/" -- the evidence-title folder and the entry, nothing
    # else in between (no "unassigned/" wrapper, no product/framework path).
    assert entry.count("/") == 1, entry
    assert entry.startswith("console-screenshot/01-console-screenshot-"), entry


def test_ordering_follows_sort_order_not_id(db_session, engineer_client, engineer_user):
    evidence = Evidence(
        title="Order check",
        file_name="ignored-primary.png",
        file_url="/uploads/ignored-primary.png",
        created_by=engineer_user.email,
    )
    db_session.add(evidence)
    db_session.flush()

    # Uploaded (and given a row, hence an id) in this order, but with
    # sort_order deliberately the other way round -- the file created
    # *second* (higher id) is given the *lower* sort_order, so an
    # implementation that accidentally orders by id instead of sort_order
    # would put the wrong file in entry "01".
    name_higher_id, url_higher_id = upload_blob("created-first.png", b"HIGHER ID LOWER SORT ORDER")
    name_lower_id, url_lower_id = upload_blob("created-second.png", b"LOWER ID HIGHER SORT ORDER")
    db_session.add(EvidenceFile(
        evidence_id=evidence.id, file_name=name_higher_id, file_url=url_higher_id, sort_order=1
    ))
    db_session.add(EvidenceFile(
        evidence_id=evidence.id, file_name=name_lower_id, file_url=url_lower_id, sort_order=0
    ))
    db_session.commit()

    response = engineer_client.get(f"/api/evidence/{evidence.id}/download")

    assert response.status_code == 200
    archive = _open_zip(response)
    entry_01 = next(n for n in archive.namelist() if n.rsplit("/", 1)[-1].startswith("01-"))
    assert archive.read(entry_01) == b"LOWER ID HIGHER SORT ORDER"


def test_mixed_png_and_pdf_keep_their_own_extensions(db_session, engineer_client):
    evidence, _files = build_evidence(
        db_session, ("shot.png", b"png bytes"), ("doc.pdf", b"pdf bytes")
    )

    response = engineer_client.get(f"/api/evidence/{evidence.id}/download")

    assert response.status_code == 200
    names = sorted(_open_zip(response).namelist())
    extensions = {n.rsplit(".", 1)[-1] for n in names}
    assert extensions == {"png", "pdf"}


def test_title_that_sanitises_to_nothing_falls_back_like_upload_does(
    db_session, engineer_client, engineer_user
):
    evidence = Evidence(
        title="!!! 日本語のみ !!!",
        file_name="ignored.png",
        file_url="/uploads/ignored.png",
        created_by=engineer_user.email,
    )
    db_session.add(evidence)
    db_session.flush()
    name, url = upload_blob("shot.png", b"fallback bytes")
    db_session.add(EvidenceFile(evidence_id=evidence.id, file_name=name, file_url=url, sort_order=0))
    db_session.commit()

    response = engineer_client.get(f"/api/evidence/{evidence.id}/download")

    assert response.status_code == 200
    [entry] = _open_zip(response).namelist()
    assert entry.startswith(f"{FALLBACK_TITLE_LABEL}/01-{FALLBACK_TITLE_LABEL}-"), entry
    # Never a leading "-" or an empty folder/label segment, the same
    # guarantee `sanitize_title` gives at upload time.
    assert not entry.startswith("-")
    assert "//" not in entry
    assert "--" not in entry

    disposition = response.headers["content-disposition"]
    assert f'filename="{FALLBACK_TITLE_LABEL}.zip"' in disposition


def test_content_disposition_filename_is_the_sanitised_title(db_session, engineer_client):
    evidence, _files = build_evidence(db_session, ("shot.png", b"cd bytes"))

    response = engineer_client.get(f"/api/evidence/{evidence.id}/download")

    assert response.status_code == 200
    assert response.headers["content-disposition"] == 'attachment; filename="console-screenshot.zip"'


# --- Reaching the browser across origins -----------------------------------
#
# The webapp can be served from its own origin, addressing the backend by an
# absolute URL with no proxy in between (see `webapp/src/api/client.ts` and
# issue #90). That makes the download cross-origin, and CORS hides every
# response header from the page except a short safe-list that
# `Content-Disposition` is not on. Without `expose_headers` the browser reads
# no filename and saves the archive as `evidence-{id}.zip`, silently undoing
# the naming the rest of this module exists to get right.


def test_content_disposition_is_exposed_to_a_cross_origin_caller(db_session, engineer_client):
    evidence, _files = build_evidence(db_session, ("shot.png", b"cors bytes"))

    response = engineer_client.get(
        f"/api/evidence/{evidence.id}/download",
        headers={"Origin": "http://localhost:5173"},
    )

    assert response.status_code == 200
    exposed = response.headers["access-control-expose-headers"]
    assert "Content-Disposition" in exposed


# --- Streaming -------------------------------------------------------------
#
# The archive is spooled and streamed rather than returned as one bytes
# object, so a large collection cannot pin the whole zip in a worker. The
# response must still be byte-identical and still declare its length, so the
# browser shows a real progress bar.


def test_streamed_archive_declares_its_length_and_matches_the_body(db_session, engineer_client):
    evidence, _files = build_evidence(
        db_session,
        ("one.png", b"first file bytes"),
        ("two.png", b"second file bytes"),
    )

    response = engineer_client.get(f"/api/evidence/{evidence.id}/download")

    assert response.status_code == 200
    assert int(response.headers["content-length"]) == len(response.content)
    archive = _open_zip(response)
    assert archive.testzip() is None
    assert len(archive.namelist()) == 2
