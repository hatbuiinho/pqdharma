import io
import tempfile
import json
import unittest
import wave
from pathlib import Path
from unittest.mock import patch
from pdf_tts import prepare_pdf_audio, align_pdf_turns


def wav_bytes():
    output = io.BytesIO()
    with wave.open(output, 'wb') as audio:
        audio.setparams((1, 2, 24000, 0, 'NONE', 'not compressed'))
        audio.writeframes(b'\0\0' * 24000)
    return output.getvalue()


class Progress:
    def report(self, *_):
        pass


class PDFSpeechTest(unittest.TestCase):
    def test_retry_reuses_completed_turns_and_preserves_gaps(self):
        spec = {'turns': [{'speaker': 'woman', 'text': 'Hello.'}, {'speaker': 'man', 'text': 'Good afternoon.'}],
                'voices': {'woman': 'bf_emma', 'man': 'bm_george'}, 'speed': 1, 'gap': .5}
        requests = []
        def response(req, **_):
            requests.append(json.loads(req.data))
            if len(requests) == 2:
                raise OSError('interrupted server')
            return io.BytesIO(wav_bytes())
        def encode(args, **_):
            Path(args[-1]).write_bytes(b'mock encoded audio')
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'recording.mp3'
            with patch('urllib.request.urlopen', side_effect=response), patch('subprocess.run', side_effect=encode):
                with self.assertRaises(RuntimeError):
                    prepare_pdf_audio(path, spec, Progress())
                self.assertFalse(path.exists())
                result = prepare_pdf_audio(path, spec, Progress())
                self.assertEqual([r['voice'] for r in requests], ['bf_emma', 'bm_george', 'bm_george'])
                self.assertEqual([(s['start'], s['end']) for s in result['segments']], [(0, 1), (1.5, 2.5)])
                self.assertEqual([s['text'] for s in result['segments']], ['Hello.', 'Good afternoon.'])
                self.assertEqual(prepare_pdf_audio(path, spec, Progress()), result)
                self.assertEqual(len(requests), 3)

    def test_sentence_splits_keep_the_original_speaker_turn(self):
        turns=[{'start': 0, 'end': 3, 'speaker': 'woman', 'text': 'Hello. Welcome!'},
               {'start': 3.5, 'end': 5, 'speaker': 'man', 'text': 'Thanks.'}]
        def align(turn):
            return [{'words': [{'word': word}]} for word in turn['text'].split()]
        result=align_pdf_turns(turns, align, Progress())
        self.assertEqual(len(result), 2)
        self.assertEqual(result[0]['text'], 'Hello. Welcome!')
        self.assertEqual(result[0]['words'], [{'word': 'Hello.'}, {'word': 'Welcome!'}])
        self.assertEqual(result[1]['start'], 3.5)
        self.assertEqual(result[1]['speaker'], 'man')

    def test_invalid_audio_is_not_checkpointed(self):
        spec = {'turns': [{'speaker': 'woman', 'text': 'Hello.'}],
                'voices': {'woman': 'bf_emma'}, 'speed': 1, 'gap': 0}
        with tempfile.TemporaryDirectory() as directory, patch('urllib.request.urlopen', return_value=io.BytesIO(b'not audio')):
            path = Path(directory) / 'recording.mp3'
            with self.assertRaises(wave.Error):
                prepare_pdf_audio(path, spec, Progress())
            self.assertFalse(path.exists())
            self.assertFalse(list(Path(directory).rglob('*.wav')))


if __name__ == '__main__':
    unittest.main()
