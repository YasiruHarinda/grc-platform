"""
Proves `db_session`'s per-test isolation actually holds even though route
handlers call `db.commit()` themselves (see conftest.py's `db_session`
fixture docstring for how the SAVEPOINT-restart pattern makes that safe).

Order matters here: if isolation were broken, the second test would see the
row the first one committed.
"""
from app.models.product import Product


def test_step1_inserts_and_commits_a_row(db_session):
    db_session.add(Product(name="Isolation Test Product"))
    db_session.commit()

    assert db_session.query(Product).count() == 1


def test_step2_does_not_see_the_previous_tests_committed_row(db_session):
    assert db_session.query(Product).count() == 0
