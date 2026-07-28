FROM python:3.12-slim

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
