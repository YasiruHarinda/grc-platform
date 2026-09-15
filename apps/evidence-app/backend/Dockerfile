FROM python:3.12-slim

# The base image is assembled from a Debian snapshot, so it carries whatever
# perl-base was current on that snapshot date rather than today's. Debian has
# since published 5.40.1-6+deb13u1, which fixes three CVEs the Choreo build
# scan treats as CRITICAL and fails the build on. Nothing in this app runs
# perl; it is simply part of the base OS. Upgrading that one package clears
# the scan and leaves libc, bash and every Python dependency untouched.
# Delete this once python:3.12-slim itself ships the fixed perl.
RUN apt-get update \
    && apt-get install -y --only-upgrade perl-base \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY . .

# Create a non-root user and transfer ownership (satisfies CKV_DOCKER_3)
RUN useradd -r -u 10014 -g root appuser \
    && chown -R appuser:root /app

USER 10014

EXPOSE 8000

CMD ["sh", "-c", "alembic upgrade head && uvicorn app.main:app --host 0.0.0.0 --port 8000"]
