"""Private, sequential Kokoro CPU synthesis; no document is sent outside this service."""
import io
import json
import logging
import math
import os
from pathlib import Path
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import urllib.request
import threading

VOICES = {'bf_emma': 'en-gb', 'bf_isabella': 'en-gb', 'bm_george': 'en-gb',
          'bm_lewis': 'en-gb', 'af_heart': 'en-us', 'am_michael': 'en-us'}
MODEL_FILES = {'kokoro-v1.0.int8.onnx': 92361271, 'voices-v1.0.bin': 28214398}
MODEL_BASE = 'https://github.com/thewh1teagle/kokoro-onnx/releases/download/model-files-v1.0/'
model = None
synthesis_lock = threading.Lock()
logging.basicConfig(level=logging.INFO)


def load_model():
    global model
    if model is not None:
        return model
    from kokoro_onnx import Kokoro
    import onnxruntime as ort
    directory = Path(os.environ.get('MODEL_DIR', '/models'))
    directory.mkdir(parents=True, exist_ok=True)
    for name, size in MODEL_FILES.items():
        path = directory / name
        if not path.is_file() or path.stat().st_size != size:
            temporary = path.with_suffix('.download')
            logging.info('Downloading model asset %s', name)
            try:
                with urllib.request.urlopen(MODEL_BASE + name, timeout=180) as src, temporary.open('wb') as dst:
                    import shutil
                    shutil.copyfileobj(src, dst)
                if temporary.stat().st_size != size:
                    raise ValueError('Incomplete model download: ' + name)
                temporary.replace(path)
            finally:
                temporary.unlink(missing_ok=True)
    options = ort.SessionOptions()
    options.intra_op_num_threads = max(1, int(os.environ.get('CPU_THREADS', '2')))
    options.inter_op_num_threads = 1
    session = ort.InferenceSession(str(directory / 'kokoro-v1.0.int8.onnx'), options,
                                   providers=['CPUExecutionProvider'])
    model = Kokoro.from_session(session, str(directory / 'voices-v1.0.bin'))
    return model


class Handler(BaseHTTPRequestHandler):
    def send(self, status, content, mime='application/json'):
        self.send_response(status)
        self.send_header('Content-Type', mime)
        self.send_header('Content-Length', str(len(content)))
        self.end_headers()
        self.wfile.write(content)

    def do_GET(self):
        if self.path == '/health':
            self.send(200, json.dumps({'ok': True, 'modelLoaded': model is not None}).encode())
        else:
            self.send(404, b'{}')

    def do_POST(self):
        if self.path != '/synthesize':
            self.send(404, b'{}')
            return
        try:
            size = int(self.headers.get('Content-Length', '0'))
            if not 0 < size <= 32000:
                raise ValueError('Invalid request size')
            data = json.loads(self.rfile.read(size))
            text, voice = data.get('text', '').strip(), data.get('voice')
            speed = float(data.get('speed', 1))
            if not text or len(text) > 4000 or voice not in VOICES or not math.isfinite(speed) or not .75 <= speed <= 1.25:
                raise ValueError('Invalid text, voice or speed')
        except (ValueError, TypeError, AttributeError):
            self.send(400, b'{"error":"Invalid synthesis request"}')
            return
        try:
            import soundfile as sf
            import numpy as np
            with synthesis_lock:
                audio, rate = load_model().create(text, voice=voice, speed=speed, lang=VOICES[voice])
            if len(audio) == 0 or not np.isfinite(audio).all():
                raise ValueError('Model returned invalid audio')
            output = io.BytesIO()
            sf.write(output, audio, rate, format='WAV', subtype='PCM_16')
            self.send(200, output.getvalue(), 'audio/wav')
        except Exception:
            logging.exception('Synthesis failed')
            self.send(503, b'{"error":"Local voice generation failed; check TTS logs and retry"}')


if __name__ == '__main__':
    ThreadingHTTPServer(('0.0.0.0', 8000), Handler).serve_forever()
