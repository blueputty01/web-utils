FROM python:3.11-slim
ARG INSTALL_PYANNOTE=false
WORKDIR /app
ENV PYTHONPATH=/app
ENV PYTEST_ADDOPTS="-o cache_dir=/tmp/pytest_cache"
RUN useradd --create-home --uid 10001 app && mkdir -p /home/app/.cache && chown -R app:app /home/app/.cache
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt \
    && if [ "$INSTALL_PYANNOTE" = "true" ]; then pip install --no-cache-dir --extra-index-url https://download.pytorch.org/whl/cpu pyannote.audio==3.3.2; fi
COPY app ./app
COPY tests ./tests
USER app
EXPOSE 8000
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
