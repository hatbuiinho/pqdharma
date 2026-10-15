FROM denoland/deno:bin AS deno
FROM python:3.11-slim
ENV PYTHONUNBUFFERED=1 HF_HOME=/models/huggingface TORCH_HOME=/models/torch NLTK_DATA=/models/nltk TOKENIZERS_PARALLELISM=false
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg libgomp1 && rm -rf /var/lib/apt/lists/*
WORKDIR /app
RUN pip install --no-cache-dir torch==2.8.0 torchaudio==2.8.0 --index-url https://download.pytorch.org/whl/cpu
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt && pip install --no-cache-dir --no-deps whisperx==3.8.6
# Only WhisperX alignment is used; diarization/pyannote/CUDA dependencies are unnecessary.
RUN useradd -u 10001 -m worker && mkdir -p /uploads /models && chown worker:worker /uploads /models
COPY --from=deno /deno /usr/local/bin/deno
COPY . .
USER worker
CMD ["python", "worker.py"]
