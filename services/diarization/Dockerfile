FROM python:3.11-slim
ARG INSTALL_PYANNOTE=false
WORKDIR /app
ENV PYTHONPATH=/app
ENV PYTEST_ADDOPTS="-o cache_dir=/tmp/pytest_cache"
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg libsndfile1 && rm -rf /var/lib/apt/lists/* \
    && useradd --create-home --uid 10001 app && mkdir -p /home/app/.cache && chown -R app:app /home/app/.cache
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt \
    && if [ "$INSTALL_PYANNOTE" = "true" ]; then pip install --no-cache-dir --extra-index-url https://download.pytorch.org/whl/cpu pyannote.audio==4.0.7; fi
ENV MPLCONFIGDIR=/home/app/.cache/matplotlib
ENV PYTHONWARNINGS="ignore::UserWarning"
COPY app ./app
COPY tests ./tests
USER app
EXPOSE 8000
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
