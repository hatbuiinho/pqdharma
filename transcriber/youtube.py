"""Download one public English video; preserve captions and reuse completed audio."""
import json
import subprocess
import tempfile
import logging
import re
import urllib.request
from datetime import datetime, timezone
from pathlib import Path

MAX_BYTES = 200_000_000


def captions_to_segments(data, duration, rolling=True):
    segments = []
    previous_words = []
    for event in data.get('events', []):
        start = event.get('tStartMs', 0) / 1000
        words = ''.join(s.get('utf8', '') for s in event.get('segs', [])).split()
        if not words or start >= duration:
            continue
        # Rolling automatic captions repeat the tail of the preceding cue.
        overlap = 0
        for size in range(1, min(len(previous_words), len(words)) + 1) if rolling else []:
            if previous_words[-size:] == words[:size]:
                overlap = size
        previous_words = words
        words = words[overlap:]
        if not words:
            continue
        end = min(duration, start + event.get('dDurationMs', 3000) / 1000)
        if segments:
            segments[-1]['end'] = min(segments[-1]['end'], start)
        if end > start:
            segments.append({'start': start, 'end': end, 'text': ' '.join(words)})
    return [s for s in segments if s['end'] > s['start']]


def prepare_youtube(path, url, progress):
    import yt_dlp
    source_path = path.with_suffix('.source.json')
    if path.is_file():
        source = json.loads(source_path.read_text()) if source_path.is_file() else {}
        if source.get('videoId'):
            prepare_thumbnail(path, source['videoId'])
        return source
    progress.report(2, 'Đang lấy thông tin YouTube')
    with tempfile.TemporaryDirectory(dir=path.parent) as temporary:
        directory = Path(temporary)
        def hook(info):
            downloaded = info.get('downloaded_bytes', 0)
            if downloaded > MAX_BYTES:
                raise ValueError('Audio tải xuống vượt quá 200 MB')
            total = info.get('total_bytes') or info.get('total_bytes_estimate')
            progress.report(3 + min(16, 16 * downloaded / total) if total else 3,
                            'Đang tải audio YouTube')
        options = {
            'noplaylist': True, 'format': 'bestaudio/best', 'quiet': True, 'noprogress': True,
            'socket_timeout': 30, 'retries': 2, 'max_filesize': MAX_BYTES,
            'ignoreerrors': 'only_download',
            'outtmpl': str(directory / 'audio.%(ext)s'), 'progress_hooks': [hook],
            'writesubtitles': True, 'writeautomaticsub': True,
            'subtitleslangs': ['en', 'en-US', 'en-GB', 'en-orig'], 'subtitlesformat': 'json3',
            'js_runtimes': {'deno': {}},
        }
        with yt_dlp.YoutubeDL(options) as downloader:
            info = downloader.extract_info(url, download=False)
            duration = info.get('duration') or 0
            if info.get('is_live') or duration <= 0 or duration > 14400:
                raise ValueError('Chỉ nhận video đã hoàn tất, thời lượng trên 0 đến 4 giờ')
            # Select one actual English track, preferring author subtitles.
            selected = None
            origin = 'whisper'
            for field, label in [('subtitles', 'youtube_manual'), ('automatic_captions', 'youtube_auto')]:
                tracks = info.get(field) or {}
                selected = next((code for code in ['en', 'en-US', 'en-GB', 'en-orig'] if code in tracks), None)
                if selected:
                    origin = label
                    break
            downloader.params['subtitleslangs'] = [selected] if selected else []
            downloader.params['writeautomaticsub'] = origin == 'youtube_auto'
            downloader.params['writesubtitles'] = origin == 'youtube_manual'
            # Captions are optional: a subtitle rate limit must not block audio/ASR.
            if selected:
                downloader.params['skip_download'] = True
                try:
                    downloader.extract_info(url, download=True)
                except yt_dlp.utils.DownloadError:
                    pass
            downloader.params['skip_download'] = False
            downloader.params['writesubtitles'] = False
            downloader.params['writeautomaticsub'] = False
            downloaded_info = downloader.extract_info(url, download=True)
            if not downloaded_info:
                raise ValueError('Không tải được audio YouTube; hãy thử lại hoặc tải MP3 lên')
            audio = Path(downloader.prepare_filename(downloaded_info))
        if not audio.is_file():
            raise ValueError('Không tải được audio YouTube')
        progress.report(20, 'Đang chuyển audio sang MP3')
        output = directory / 'converted.mp3'
        subprocess.run(['ffmpeg', '-nostdin', '-v', 'error', '-y', '-i', str(audio),
                        '-vn', '-codec:a', 'libmp3lame', '-b:a', '192k', str(output)],
                       check=True, timeout=900, capture_output=True)
        probe = subprocess.run(['ffprobe', '-v', 'error', '-show_entries', 'format=duration',
                                '-of', 'json', str(output)], check=True, capture_output=True, timeout=30)
        actual_duration = float(json.loads(probe.stdout)['format']['duration'])
        if not 0 < actual_duration <= 14400 or output.stat().st_size > MAX_BYTES:
            raise ValueError('MP3 vượt quá giới hạn 200 MB hoặc 4 giờ')
        source = {'videoId': info['id'], 'url': url, 'title': info.get('title'),
                  'channel': info.get('channel'), 'channelId': info.get('channel_id'),
                  'importedAt': datetime.now(timezone.utc).isoformat(), 'transcriptSource': origin}
        caption_files = list(directory.glob('audio.*.json3'))
        if caption_files:
            try:
                raw = json.loads(caption_files[0].read_text())
                source['segments'] = captions_to_segments(raw, actual_duration, rolling=origin == 'youtube_auto')
                path.with_suffix('.captions.json3').write_text(json.dumps(raw, ensure_ascii=False))
            except (ValueError, TypeError, KeyError):
                source['transcriptSource'] = 'whisper'
        if not source.get('segments'):
            source['transcriptSource'] = 'whisper'
        source_path.write_text(json.dumps(source, ensure_ascii=False))
        output.replace(path)
        prepare_thumbnail(path, info['id'])
        return source


def prepare_thumbnail(path, video_id):
    """Optional bounded download from a fixed YouTube host, normalized to JPEG."""
    target = path.with_suffix('.thumbnail.jpg')
    if target.is_file() or not re.fullmatch(r'[A-Za-z0-9_-]{11}', video_id):
        return
    try:
        with tempfile.TemporaryDirectory(dir=path.parent) as directory:
            raw = Path(directory) / 'thumbnail'
            with urllib.request.urlopen(f'https://i.ytimg.com/vi/{video_id}/hqdefault.jpg', timeout=15) as response:
                data = response.read(5_000_001)
            if len(data) > 5_000_000:
                raise ValueError('Thumbnail exceeds 5 MB')
            raw.write_bytes(data)
            output = Path(directory) / 'thumbnail.jpg'
            subprocess.run(['ffmpeg', '-nostdin', '-v', 'error', '-y', '-i', str(raw),
                            '-frames:v', '1', '-vf', 'scale=640:-2', str(output)],
                           check=True, capture_output=True, timeout=30)
            output.replace(target)
    except Exception:
        logging.warning('Thumbnail unavailable for %s; audio import continues', video_id)
