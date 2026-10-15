"""Private S3 objects; local files are disposable only after verified publication."""
import hashlib
import os
import tempfile
from pathlib import Path
from urllib.parse import urlparse


class Storage:
    def __init__(self):
        endpoint = os.environ.get('S3_ENDPOINT', '')
        self.client = None
        if not endpoint:
            return
        parsed = urlparse(endpoint)
        if (parsed.scheme not in ('http', 'https') or not parsed.netloc
                or parsed.path not in ('', '/') or parsed.username
                or parsed.query or parsed.fragment):
            raise ValueError('S3_ENDPOINT must be an http(s) origin without bucket/path')
        from minio import Minio
        import urllib3
        self.bucket = os.environ['S3_BUCKET']
        self.prefix = os.environ.get('S3_PREFIX', 'prod').strip('/') or 'prod'
        if any(p in ('.', '..') for p in self.prefix.split('/')):
            raise ValueError('Invalid S3_PREFIX')
        self.client = Minio(parsed.netloc, access_key=os.environ['S3_ACCESS_KEY'],
                            secret_key=os.environ['S3_SECRET_KEY'], secure=parsed.scheme == 'https',
                            region=os.environ.get('S3_REGION') or None,
                            http_client=urllib3.PoolManager(timeout=urllib3.Timeout(connect=10, read=120),
                                                           retries=urllib3.Retry(total=2)))

    def restore(self, db, recording_id, kind, destination):
        if not self.client:
            return False
        row = db.execute('SELECT object_key,sha256 FROM recording_assets WHERE recording_id=%s AND kind=%s',
                         (recording_id, kind)).fetchone()
        if not row:
            return False
        destination = Path(destination)
        destination.parent.mkdir(parents=True, exist_ok=True)
        descriptor, temporary = tempfile.mkstemp(dir=destination.parent, suffix='.download')
        os.close(descriptor)
        try:
            self.client.fget_object(self.bucket, row['object_key'], temporary)
            if digest(temporary) != row['sha256']:
                raise ValueError('Downloaded audio/source checksum mismatch')
            os.replace(temporary, destination)
        finally:
            Path(temporary).unlink(missing_ok=True)
        return True

    def publish(self, db, recording_id, kind, filename, mime, extension):
        if not self.client:
            return
        filename = Path(filename)
        checksum = digest(filename)
        key = f'{self.prefix}/recordings/{recording_id}/{kind}/{checksum}{extension}'
        row = db.execute('SELECT sha256 FROM recording_assets WHERE recording_id=%s AND kind=%s',
                         (recording_id, kind)).fetchone()
        if row and row['sha256'] == checksum:
            return
        self.client.fput_object(self.bucket, key, str(filename), content_type=mime,
                                metadata={'sha256': checksum})
        stat = self.client.stat_object(self.bucket, key)
        if stat.size != filename.stat().st_size or stat.metadata.get('x-amz-meta-sha256') != checksum:
            raise ValueError('Uploaded object verification failed')
        db.execute('''INSERT INTO recording_assets(recording_id,kind,object_key,content_type,size_bytes,sha256)
            VALUES(%s,%s,%s,%s,%s,%s) ON CONFLICT(recording_id,kind) DO UPDATE SET
            object_key=EXCLUDED.object_key,content_type=EXCLUDED.content_type,
            size_bytes=EXCLUDED.size_bytes,sha256=EXCLUDED.sha256,updated_at=now()''',
                   (recording_id, kind, key, mime, stat.size, checksum))


def digest(filename):
    with open(filename, 'rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()
