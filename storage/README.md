# File Storage System

The file storage system supports both local filesystem (for development) and AWS S3 (for production).

## Configuration

### Environment Variables

**For Local Storage (Development):**
```bash
STORAGE_TYPE=local
STORAGE_LOCAL_PATH=./storage/files  # Optional, defaults to ./storage/files
```

**For S3 Storage (Production):**
```bash
STORAGE_TYPE=s3
AWS_S3_BUCKET=your-bucket-name
AWS_REGION=us-east-1  # Optional, defaults to us-east-1
AWS_ACCESS_KEY_ID=your-access-key  # Optional if using IAM role
AWS_SECRET_ACCESS_KEY=your-secret-key  # Optional if using IAM role
```

## Usage

### Initialize Storage

```go
import "meritdraft-backend/storage"

// From environment variables
storage, err := storage.NewStorageFromEnv()
if err != nil {
    log.Fatal(err)
}

// Or with explicit configuration
cfg := storage.StorageConfig{
    Type:      storage.StorageTypeS3,
    S3Bucket:  "my-bucket",
    S3Region:  "us-east-1",
}
storage, err := storage.NewStorage(cfg)
```

### Upload a File

```go
fileID := uuid.New()
filename := "cv.pdf"
fileData := // io.Reader from multipart form or file

storagePath, err := storage.Upload(ctx, fileID, filename, fileData)
if err != nil {
    return err
}

// Store storagePath in database File record
```

### Download a File

```go
// Get storagePath from database File record
reader, err := storage.Download(ctx, storagePath)
if err != nil {
    return err
}
defer reader.Close()

// Stream to client or process file
io.Copy(responseWriter, reader)
```

### Delete a File

```go
err := storage.Delete(ctx, storagePath)
if err != nil {
    return err
}
```

## Storage Path Format

Files are stored with the following path structure:
```
{first-2-chars-of-uuid}/{uuid}_{sanitized-filename}.{ext}
```

Example:
```
12/123e4567-e89b-12d3-a456-426614174000_Dr_Jane_Smith_CV.pdf
```

This structure:
- Prevents too many files in a single directory
- Ensures uniqueness using UUID
- Maintains original filename for reference

## S3 Setup

### 1. Create S3 Bucket

```bash
aws s3 mb s3://meritdraft-files --region us-east-1
```

### 2. Configure Bucket Policy (Optional - for public access to specific files)

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowGetObject",
      "Effect": "Allow",
      "Principal": "*",
      "Action": "s3:GetObject",
      "Resource": "arn:aws:s3:::meritdraft-files/*"
    }
  ]
}
```

### 3. Set Environment Variables

Add to your `.env` file:
```bash
STORAGE_TYPE=s3
AWS_S3_BUCKET=meritdraft-files
AWS_REGION=us-east-1
AWS_ACCESS_KEY_ID=your-key
AWS_SECRET_ACCESS_KEY=your-secret
```

### 4. IAM Permissions

If using IAM roles (recommended for production), ensure the role has:
- `s3:PutObject`
- `s3:GetObject`
- `s3:DeleteObject`

## Local Storage Setup

For development, local storage is the default:

```bash
STORAGE_TYPE=local
STORAGE_LOCAL_PATH=./storage/files
```

The directory structure will be created automatically.

## Integration with File Repository

The storage system works with the `FileRepository`:

```go
// 1. Upload file to storage
storagePath, err := storage.Upload(ctx, fileID, filename, fileData)

// 2. Create file record in database
file := &models.File{
    ID:          fileID,
    UserID:      userID,
    PetitionID:  &petitionID,
    Filename:    filename,
    MimeType:    mimeType,
    Size:        size,
    StoragePath: storagePath,
}

err = fileRepo.Create(ctx, file)

// 3. Later, retrieve file
file, err := fileRepo.GetByID(ctx, fileID)
reader, err := storage.Download(ctx, file.StoragePath)
```

## Security Considerations

1. **File Validation**: Validate file type and size before upload
2. **Access Control**: Ensure users can only access their own files
3. **Virus Scanning**: Consider adding virus scanning for production
4. **Encryption**: S3 supports server-side encryption (SSE)
5. **Presigned URLs**: For direct client access, consider using S3 presigned URLs

