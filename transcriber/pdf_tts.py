"""Synthesize reviewed turns locally, checkpoint each turn, and retain exact boundaries."""
import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import urllib.request
import wave


def prepare_pdf_audio(path, specification, progress):
    fingerprint = hashlib.sha256(json.dumps(specification, sort_keys=True, ensure_ascii=False).encode()).hexdigest()
    sidecar = path.with_suffix('.source.json')
    if path.is_file() and sidecar.is_file():
        source = json.loads(sidecar.read_text())
        if source.get('fingerprint') == fingerprint:
            return source
    directory = path.parent / (path.stem + '.tts-cache')
    directory.mkdir(exist_ok=True)
    turns = specification['turns']
    origin = os.environ.get('TTS_ORIGIN', 'http://pq-dharma-tts:8000').rstrip('/')
    output_path = directory / 'joined.wav'
    segments, position = [], 0
    rate = None
    try:
        with wave.open(str(output_path), 'wb') as output:
            for i, turn in enumerate(turns):
                progress.report(2 + 58*i/len(turns), f'Đang tạo giọng đọc local · lượt {i+1}/{len(turns)}')
                request_data = {'text': turn['text'], 'voice': specification['voices'][turn['speaker']],
                                'speed': specification['speed']}
                key = hashlib.sha256(json.dumps(request_data, sort_keys=True).encode()).hexdigest()
                cached = directory / (key + '.wav')
                if not cached.is_file():
                    req = urllib.request.Request(origin + '/synthesize', data=json.dumps(request_data).encode(),
                                                 headers={'Content-Type': 'application/json'}, method='POST')
                    try:
                        with urllib.request.urlopen(req, timeout=900) as response:
                            raw = response.read(32_000_001)
                        if len(raw) > 32_000_000:
                            raise ValueError('Một lượt đọc quá dài; hãy chia nhỏ nội dung')
                    except Exception as exc:
                        raise RuntimeError('Không tạo được giọng local. Kiểm tra service tts rồi bấm Thử lại.') from exc
                    temporary = cached.with_suffix('.tmp')
                    temporary.write_bytes(raw)
                    # Validate before the cache becomes visible to retries.
                    with wave.open(io.BytesIO(raw), 'rb') as check:
                        if check.getnchannels() != 1 or check.getsampwidth() != 2 or check.getnframes() <= 0:
                            raise ValueError('Audio TTS không hợp lệ')
                    temporary.replace(cached)
                with wave.open(str(cached), 'rb') as audio:
                    if rate is None:
                        rate = audio.getframerate()
                        output.setparams((1, 2, rate, 0, 'NONE', 'not compressed'))
                    if audio.getframerate() != rate or audio.getnchannels() != 1 or audio.getsampwidth() != 2:
                        raise ValueError('Định dạng các lượt đọc không đồng nhất')
                    frames = audio.getnframes()
                    start = position / rate
                    output.writeframes(audio.readframes(frames))
                    position += frames
                    segments.append({'start': start, 'end': position/rate, 'text': turn['text'],
                                     'speaker': turn['speaker']})
                    if i + 1 < len(turns):
                        gap_frames = round(specification['gap'] * rate)
                        output.writeframes(b'\0\0' * gap_frames)
                        position += gap_frames
                if position / rate > 14400:
                    raise ValueError('Audio vượt quá giới hạn 4 giờ')
        progress.report(62, 'Đang ghép và lưu audio hội thoại')
        temporary_mp3 = directory / 'audio.mp3'
        subprocess.run(['ffmpeg', '-nostdin', '-y', '-v', 'error', '-i', str(output_path),
                        '-codec:a', 'libmp3lame', '-b:a', '192k', str(temporary_mp3)], check=True, timeout=300)
        if temporary_mp3.stat().st_size > 200_000_000:
            raise ValueError('Audio vượt quá 200 MB; hãy chia nội dung thành các phần nhỏ')
        source = {'origin': 'pdf_tts', 'model': 'Kokoro-82M-v1.0-int8', 'fingerprint': fingerprint,
                  'filename': specification.get('filename', ''), 'voices': specification['voices'],
                  'speed': specification['speed'], 'gap': specification['gap'], 'segments': segments}
        temporary_source = directory / 'source.json'
        temporary_source.write_text(json.dumps(source, ensure_ascii=False))
        temporary_mp3.replace(path)
        temporary_source.replace(sidecar)
        return source
    finally:
        output_path.unlink(missing_ok=True)


def clean_pdf_cache(path):
    shutil.rmtree(path.parent / (path.stem + '.tts-cache'), ignore_errors=True)


def align_pdf_turns(turns, align, progress):
    result = []
    for i, turn in enumerate(turns):
        sentences = align(turn)
        words = [word for sentence in sentences for word in sentence.get('words', [])]
        if not words:
            raise ValueError('Không căn được lời cho một lượt đọc; hãy kiểm tra và thử lại')
        result.append({**turn, 'words': words})
        progress.report(70 + 20*(i+1)/len(turns), f'Đã căn thời gian lượt {i+1}/{len(turns)}')
    return result
