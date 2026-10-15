import unittest
from youtube import captions_to_segments

class CaptionTests(unittest.TestCase):
    def test_rolling_captions_and_bounds(self):
        data = {'events': [
            {'tStartMs': 0, 'dDurationMs': 3000, 'segs': [{'utf8': 'one two'}]},
            {'tStartMs': 1000, 'dDurationMs': 3000, 'segs': [{'utf8': 'two three'}]},
            {'tStartMs': 2000, 'segs': [{'utf8': '\n'}]},
            {'tStartMs': 5000, 'segs': [{'utf8': 'outside'}]},
        ]}
        result = captions_to_segments(data, 3)
        self.assertEqual(result, [
            {'start': 0, 'end': 1, 'text': 'one two'},
            {'start': 1, 'end': 3, 'text': 'three'},
        ])

    def test_missing_captions_fall_back(self):
        self.assertEqual(captions_to_segments({}, 20), [])

    def test_author_captions_keep_repeated_words(self):
        data = {'events': [
            {'tStartMs': 0, 'dDurationMs': 1000, 'segs': [{'utf8': 'yes'}]},
            {'tStartMs': 1000, 'dDurationMs': 1000, 'segs': [{'utf8': 'yes'}]},
        ]}
        self.assertEqual(len(captions_to_segments(data, 3, rolling=False)), 2)

    def test_subtitle_failure_falls_back_and_completed_audio_is_reused(self):
        import json
        import sys
        import tempfile
        from pathlib import Path
        from types import SimpleNamespace
        from unittest.mock import patch
        from youtube import prepare_youtube

        class DownloadError(Exception):
            pass

        calls = []
        class Downloader:
            def __init__(self, options):
                self.params = options
            def __enter__(self):
                return self
            def __exit__(self, *args):
                pass
            def extract_info(self, url, download):
                calls.append((download, self.params.get('skip_download', False)))
                if download and self.params.get('skip_download'):
                    raise DownloadError('subtitle HTTP 429')
                info = {'id': 'aaaS89avDVU', 'duration': 10, 'subtitles': {'en': [{}]}}
                if download:
                    Path(self.params['outtmpl'].replace('%(ext)s', 'm4a')).write_bytes(b'audio')
                return info
            def prepare_filename(self, info):
                return self.params['outtmpl'].replace('%(ext)s', 'm4a')

        def run(args, **kwargs):
            if args[0] == 'ffmpeg':
                Path(args[-1]).write_bytes(b'mp3')
            return SimpleNamespace(stdout=json.dumps({'format': {'duration': '10'}}).encode())

        fake = SimpleNamespace(YoutubeDL=Downloader, utils=SimpleNamespace(DownloadError=DownloadError))
        progress = SimpleNamespace(report=lambda *args: None)
        with tempfile.TemporaryDirectory() as directory, patch.dict(sys.modules, {'yt_dlp': fake}), patch('youtube.subprocess.run', side_effect=run), patch('youtube.prepare_thumbnail'):
            path = Path(directory) / 'test.mp3'
            source = prepare_youtube(path, 'url', progress)
            self.assertTrue(path.is_file())
            self.assertEqual(source['transcriptSource'], 'whisper')
            self.assertEqual(calls, [(False, False), (True, True), (True, False)])
            calls.clear()
            self.assertEqual(prepare_youtube(path, 'url', progress), source)
            self.assertEqual(calls, [])
