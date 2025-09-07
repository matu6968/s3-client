# S3 Client

This is a simple command-line tool to interact with an S3-compatible storage service. It allows you to upload, list files from an S3 bucket.

## Prerequisites

- Go (1.23 or later)

## Installation

1. Clone the repository:
   ```
   git clone https://github.com/matu6968/s3-client
   ```

2. Go to the project directory:
   ```
   cd s3-client
   ```

3. Build the binary:
   ```
   go build -o s3-client
   ```

## Configuration

Create a configuration file `s3config.toml` with the following content:

```
access_key_id = "your_access_key_id"
secret_access_key = "your_secret_access_key"
region = "your_region"
bucket = "your_bucket_name"
endpoint = "your_endpoint_url"

returnurl = "your_return_url"
```

## Usage

### Upload a file

```
./s3-client_linux.x86_64 -file "path/to/your/file" [optional] -directory "/exampledir"
```

### List files

```
./s3-client_linux.x86_64 -list
```

### Delete files

```
./s3-client_linux.x86_64 -delete "filename.png"

or with an directory

./s3-client_linux.x86_64 -delete "/dir1/filename.png"
```

### Help message

```
./s3-client -help
```
