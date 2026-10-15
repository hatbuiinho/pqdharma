import unittest
from normalization import normalize_segment, normalize_segments


class AlignmentTests(unittest.TestCase):
    def test_missing_and_overlapping_times_are_flagged(self):
        segment = {'text':'one two three four', 'start':1, 'end':2, 'words':[
            {'word':'one','start':1.1,'end':1.3}, {'word':'two'},
            {'word':'three','start':1.6,'end':1.8}, {'word':'four','start':1.7,'end':2.1}]}
        result = normalize_segment(segment)
        self.assertEqual([t['text'] for t in result['tokens']], segment['text'].split())
        previous=1000
        for token in result['tokens']:
            self.assertGreaterEqual(token['startMs'],previous)
            self.assertGreater(token['endMs'],token['startMs'])
            self.assertLessEqual(token['endMs'],2000)
            previous=token['endMs']
        self.assertFalse(result['tokens'][0]['endEstimated'])
        self.assertTrue(result['tokens'][1]['endEstimated'])
        self.assertTrue(result['tokens'][3]['endEstimated'])

    def test_edit_keeps_original_bounds(self):
        s=normalize_segment({'text':'new words','start':0,'end':1},7040,9520)
        self.assertEqual((s['startMs'],s['endMs']),(7040,9520))
        self.assertTrue(all(t['endEstimated'] for t in s['tokens']))

    def test_empty_and_too_short_audio_fail(self):
        with self.assertRaises(ValueError): normalize_segments([])
        with self.assertRaises(ValueError): normalize_segment({'text':'too many words','start':0,'end':.001})


if __name__=='__main__': unittest.main()
