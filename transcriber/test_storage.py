import hashlib
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import Mock
from storage import Storage


class StorageTests(unittest.TestCase):
    def make_store(self):
        storage = Storage.__new__(Storage)
        storage.client = Mock()
        storage.bucket, storage.prefix = 'private', 'prod'
        return storage

    def test_corrupt_download_does_not_replace_cache(self):
        storage = self.make_store()
        db = Mock()
        db.execute.return_value.fetchone.return_value = {'object_key': 'audio', 'sha256': 'wrong'}
        storage.client.fget_object.side_effect = lambda bucket, key, name: Path(name).write_bytes(b'corrupt')
        with tempfile.TemporaryDirectory() as directory:
            destination = Path(directory) / 'audio.mp3'
            destination.write_bytes(b'previous')
            with self.assertRaisesRegex(ValueError, 'checksum'):
                storage.restore(db, 'recording', 'audio', destination)
            self.assertEqual(destination.read_bytes(), b'previous')
            self.assertEqual(list(Path(directory).iterdir()), [destination])

    def test_failed_upload_verification_does_not_register_object(self):
        storage = self.make_store()
        db = Mock()
        db.execute.return_value.fetchone.return_value = None
        storage.client.stat_object.return_value = SimpleNamespace(size=3, metadata={})
        with tempfile.TemporaryDirectory() as directory:
            filename = Path(directory) / 'audio.mp3'
            filename.write_bytes(b'mp3')
            with self.assertRaisesRegex(ValueError, 'verification'):
                storage.publish(db, 'recording', 'audio', filename, 'audio/mpeg', '.mp3')
        self.assertEqual(db.execute.call_count, 1)

    def test_verified_upload_is_registered_with_content_hash(self):
        storage = self.make_store()
        db = Mock()
        db.execute.return_value.fetchone.return_value = None
        checksum = hashlib.sha256(b'mp3').hexdigest()
        storage.client.stat_object.return_value = SimpleNamespace(size=3, metadata={'x-amz-meta-sha256': checksum})
        with tempfile.TemporaryDirectory() as directory:
            filename = Path(directory) / 'audio.mp3'
            filename.write_bytes(b'mp3')
            storage.publish(db, 'recording', 'audio', filename, 'audio/mpeg', '.mp3')
        self.assertEqual(db.execute.call_count, 2)
        self.assertEqual(db.execute.call_args.args[1], ('recording', 'audio',
                         f'prod/recordings/recording/audio/{checksum}.mp3', 'audio/mpeg', 3, checksum))


class LiveStorageTests(unittest.TestCase):
    @unittest.skipUnless(__import__('os').environ.get('TEST_S3_EXISTING_BUCKET') == '1'
                         and __import__('os').environ.get('TEST_DATABASE_URL'),
                         'set TEST_S3_EXISTING_BUCKET and TEST_DATABASE_URL for real S3/DB checks')
    def test_publish_restore_cycle(self):
        import os
        import uuid
        import psycopg
        from psycopg import sql
        from psycopg.rows import dict_row
        storage = Storage()
        recording_id = 'storage-check-' + uuid.uuid4().hex
        schema = 'test_storage_' + uuid.uuid4().hex
        with psycopg.connect(os.environ['TEST_DATABASE_URL'], autocommit=True, row_factory=dict_row) as db:
            db.execute(sql.SQL('CREATE SCHEMA {}').format(sql.Identifier(schema)))
            keys = []
            try:
                db.execute(sql.SQL('SET search_path TO {}').format(sql.Identifier(schema)))
                for migration in sorted((Path(__file__).resolve().parents[1] / 'be/migrations').glob('*.sql')):
                    db.execute(migration.read_text())
                db.execute("INSERT INTO sutras VALUES(%s,'storage test','en')", (recording_id,))
                db.execute("INSERT INTO recordings(id,sutra_id,title,audio_file,source_url,status) VALUES(%s,%s,'test','audio.mp3','','ready')",
                           (recording_id, recording_id))
                with tempfile.TemporaryDirectory() as directory:
                    for kind, payload, mime, extension in [
                        ('audio', b'mp3 bytes', 'audio/mpeg', '.mp3'),
                        ('source', b'{"videoId":"test"}', 'application/json', '.json'),
                    ]:
                        filename = Path(directory) / ('original' + extension)
                        filename.write_bytes(payload)
                        storage.publish(db, recording_id, kind, filename, mime, extension)
                        row = db.execute('SELECT object_key FROM recording_assets WHERE recording_id=%s AND kind=%s',
                                         (recording_id, kind)).fetchone()
                        keys.append(row['object_key'])
                        filename.unlink()
                        self.assertTrue(storage.restore(db, recording_id, kind, filename))
                        self.assertEqual(filename.read_bytes(), payload)
            finally:
                for key in keys:
                    storage.client.remove_object(storage.bucket, key)
                db.execute(sql.SQL('DROP SCHEMA {} CASCADE').format(sql.Identifier(schema)))
