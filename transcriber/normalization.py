"""Convert alignment output to the player's strict, non-overlapping token schema."""
import math


def milliseconds(value):
    return round(float(value) * 1000)


def normalize_segment(segment, start_ms=None, end_ms=None):
    text = segment['text'].strip()
    words = segment.get('words') or [{'word': word} for word in text.split()]
    words = [w for w in words if w.get('word', '').strip()]
    if not words:
        raise ValueError('Không có lời trong đoạn audio')
    start = milliseconds(segment['start']) if start_ms is None else start_ms
    end = milliseconds(segment['end']) if end_ms is None else end_ms
    if end - start < len(words):
        raise ValueError('Đoạn audio quá ngắn để căn thời gian từng từ')
    tokens = []
    previous = start
    for i, word in enumerate(words):
        remaining = len(words) - i - 1
        limit = end - remaining
        raw_start, raw_end = word.get('start'), word.get('end')
        known = raw_start is not None and raw_end is not None and not word.get("estimated", False)
        if known:
            known = math.isfinite(float(raw_start)) and math.isfinite(float(raw_end))
        if known:
            requested_start = milliseconds(raw_start)
            requested_end = milliseconds(raw_end)
        else:
            # Use the next acoustic anchor when possible; explicitly mark estimates.
            next_start = next((milliseconds(w['start']) for w in words[i+1:]
                               if w.get('start') is not None and math.isfinite(float(w['start'])) and not w.get('estimated', False)), end)
            missing_count = next((j+1 for j, w in enumerate(words[i+1:])
                                  if w.get('start') is not None and math.isfinite(float(w['start'])) and not w.get('estimated', False)), remaining+1)
            requested_start = previous
            requested_end = previous + max(1, (next_start - previous) // missing_count)
        token_start = max(previous, min(requested_start, limit-1))
        token_end = max(token_start+1, min(requested_end, limit))
        estimated = not known or token_start != requested_start or token_end != requested_end
        tokens.append({'text': word['word'].strip(), 'startMs': token_start,
                       'endMs': token_end, 'endEstimated': estimated})
        previous = token_end
    return {'text': text, 'startMs': start, 'endMs': end, 'tokens': tokens}


def normalize_segments(segments):
    result = []
    previous = 0
    for segment in segments:
        if not segment['text'].strip():
            continue
        start = max(previous, milliseconds(segment['start']))
        end = milliseconds(segment['end'])
        result.append(normalize_segment(segment, start, end))
        previous = end
    if not result:
        raise ValueError('Không nhận dạng được lời nói tiếng Anh trong audio')
    return result
