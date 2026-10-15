FROM python:3.11-slim
ENV PYTHONUNBUFFERED=1 OMP_NUM_THREADS=2 MODEL_DIR=/models
RUN apt-get update && apt-get install -y --no-install-recommends libsndfile1 espeak-ng ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
RUN useradd -u 10001 -m tts && mkdir -p /models && chown tts:tts /models
COPY server.py .
USER tts
EXPOSE 8000
CMD ["python", "server.py"]
