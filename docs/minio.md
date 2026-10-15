# MinIO storage

## Checklist triển khai

- [x] Cấu hình S3 ở API và Python worker, giữ chế độ local nếu endpoint trống.
- [x] Migration 005: metadata object và con trỏ bản xuất transcript.
- [x] MP3 upload được kiểm tra rồi lưu MinIO trước khi tạo job.
- [x] YouTube: audio, provenance JSON, captions JSON3, thumbnail JPEG.
- [x] API stream private object, hỗ trợ Range/HEAD/If-Range.
- [x] Worker khôi phục cache có kiểm tra SHA-256; chỉ xóa cache đã publish thành công.
- [x] Reconciliation theo chu kỳ, xuất JSON độc lập, retry sau restart.
- [x] Lệnh chuyển dữ liệu cũ, giữ nguyên file gốc.
- [ ] Backup PostgreSQL và bucket ra nơi độc lập: cấu hình theo hạ tầng vận hành.

## Cấu hình

Điền `.env` (được gitignore):

```dotenv
S3_ENDPOINT=https://minio-api.hatbuinho.me
S3_BUCKET=pqdharma
S3_PREFIX=prod
S3_ACCESS_KEY=<access key>
S3_SECRET_KEY=<secret key>
S3_REGION=us-east-1
```

Endpoint không chứa tên bucket. SDK luôn dùng path-style. Không đưa credentials vào frontend. Bucket phải private; ứng dụng không tự tạo bucket hoặc sửa bucket policy. Nếu bật endpoint mà thiếu credential, API dừng với lỗi cấu hình.

```sh
docker compose up -d --build api transcriber web
```

Không xóa volume `audio_uploads` trước khi kiểm tra đồng bộ toàn bộ dữ liệu cũ.

## Object layout

```text
prod/recordings/{recordingId}/audio/{sha256}.mp3
prod/recordings/{recordingId}/thumbnails/{sha256}.jpg
prod/recordings/{recordingId}/source/{sha256}.json
prod/recordings/{recordingId}/captions/{sha256}.json3
prod/recordings/{recordingId}/exports/transcript-r{revision}-{sha256}.json
```

Database `recording_assets` giữ object key, MIME, byte size, SHA-256. S3 user metadata cũng giữ SHA-256. Upload được xác nhận bằng stat size và checksum metadata; download về cache kiểm tra lại SHA-256 từ nội dung.

PostgreSQL là nguồn chính cho transcript và IPA. Bản xuất lấy trong transaction repeatable-read và được fingerprint theo toàn bộ nội dung, nên thay đổi IPA/default accent vẫn tạo bản xuất mới ngay cả khi transcript revision không tăng. `storage_exports` trỏ tới bản đã đồng bộ gần nhất. Các bản cũ vẫn được giữ.

## Chuyển file cũ và retry

API tự quét dữ liệu sau startup và mỗi phút. Bài có job đang chạy được bỏ qua cho đến khi hoàn tất. Worker publish audio và sidecar trước khi nhận dạng. Thumbnail lỗi được retry riêng khi worker rảnh; các bài YouTube cũ dùng URL nguồn để bổ sung thumbnail. Khi lỗi lưu MinIO, file local được giữ để retry. Lỗi export được log và retry riêng, không đổi trạng thái job nhận dạng.

Chạy đồng bộ một lần và kiểm tra exit code:

```sh
docker compose run --rm --no-deps api ./server \
  -sample /data/import.json -auto-ipa=false -sync-storage-once
```

Lệnh không chạy HTTP server và không xóa file local. Chỉ báo thành công khi không có lỗi được phát hiện; bài đang có job cần đợi xong rồi chạy lại. Sample audio và captions gốc cũng được chuyển. Không tự động xóa audio/source trên MinIO.

API ưu tiên MinIO khi đã có object key; bài cũ chưa chuyển tiếp tục đọc local. Worker tải lại audio/source khi cache thiếu. Các object không còn được tham chiếu do transaction lỗi hoặc import thất bại cần được kiểm kê trước khi dọn; không đặt lifecycle xóa toàn bộ prefix prod.

Bản xuất phản ánh trạng thái mới nhất khi đồng bộ, không phải nhật ký lưu mọi chỉnh sửa trung gian. Chưa có cơ chế xóa bài khỏi UI, nên việc xóa row DB không tự xóa object MinIO.

## Quyền tối thiểu

User/service account chỉ cần `s3:GetObject`, `s3:PutObject`, `s3:AbortMultipartUpload`, `s3:ListMultipartUploadParts` trên `arn:aws:s3:::pqdharma/prod/*`; quyền bucket cần `s3:GetBucketLocation`, `s3:ListBucket`, `s3:ListBucketMultipartUploads` (giới hạn prefix nếu tương thích client). Không cần quyền quản trị hoặc tạo bucket. Quyền `s3:DeleteObject` chỉ cần cho dọn object; smoke test có cleanup cũng cần quyền này.

Với staging, dùng prefix và credential riêng. API proxy cùng origin nên không cần CORS cho trình duyệt ở giai đoạn này. Không dùng URL ký trong DB.

## Vận hành

- Backup PostgreSQL và MinIO ra đích độc lập; kiểm tra phục hồi định kỳ.
- Có thể bật versioning theo nhu cầu dung lượng; không thay thế backup.
- Đặt lifecycle dọn incomplete multipart upload, không expire audio/source đang sử dụng.
- Theo dõi log API (`storage sync`, `storage export`) và worker khi upload/download lỗi.
- Khi thay đổi object ngoài ứng dụng, cần kiểm kê và đồng bộ lại metadata; không sửa đè object có key SHA-256.

## Kiểm tra

```sh
cd be
go test ./...
go vet ./...
# TEST_DATABASE_URL=... go test ./... để chạy kiểm tra DB schema riêng
```

```sh
python3 -m unittest discover -s transcriber
npm --prefix fe run check
npm --prefix fe test
npm --prefix fe run build
```

Go streaming test opt-in: `TEST_S3_EXISTING_BUCKET=1` cùng các biến S3 chạy `go test ./internal/objectstore -run TestMinIOStream -v -count=1` trong thư mục be. Test tạo object có recording ID bắt đầu bằng storage-check-, kiểm tra upload/range/private access rồi xóa object đó; không sửa policy.

## Kết quả xác thực triển khai ngày 15/10/2026

- Server MinIO thật: upload, stat/checksum metadata, đọc lại, Range 206, range lỗi 416, HEAD, If-Range, chặn anonymous GET và cleanup object thử nghiệm đều qua.
- Go + PostgreSQL schema thử nghiệm riêng: asset registration, upload idempotent, xuất JSON nhất quán và làm mới khi đổi IPA đều qua.
- Python + MinIO thật: publish audio/source, xóa cache, restore và kiểm tra byte nội dung đều qua.
- 4 bài sẵn có: đồng bộ 4 audio (~199 MB), 4 transcript JSON; nguồn/captions có sẵn được chuyển, thumbnail YouTube được bổ sung.
- Audio Range và thumbnail đã được kiểm tra qua frontend proxy tại localhost:3000.
- Kiểm tra end-to-end bằng MP3 thử: upload → MinIO → nhận dạng → căn thời gian → IPA thành công. Cache audio được xóa; yêu cầu sửa lời tiếp theo tải lại audio từ MinIO và hoàn tất. Dữ liệu thử và object thử đã được dọn.
- Go tests/vet, 11 Python tests (gồm MinIO/DB thật), frontend check, 13 frontend tests và production build đều qua.
