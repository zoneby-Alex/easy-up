# easy-up

easy-up is a lightweight LAN image transfer tool built with Go. It provides a browser-based interface for uploading, browsing, downloading, and deleting image files on a local network.

## Features

- Upload images by drag and drop or file picker.
- Browse uploaded files and folders.
- Create folders.
- Download files from the browser.
- Delete files or folders when enabled.
- Configure port, upload directory, file size limit, and page title.
- Print detected LAN access URLs on startup.

## Requirements

- Go 1.26.1 or compatible version

## Run

```powershell
go run .
```

Then open:

```text
http://localhost:8080
```

The server also prints detected LAN addresses that can be used by other devices on the same network.

## Build

```powershell
go build -o easy-up.exe .
```

## Configuration

Edit `config.json`:

```json
{
  "port": "8080",
  "upload_dir": "./uploads",
  "max_file_size": 52428800,
  "allow_deletes": true,
  "title": "局域网图片传输工具"
}
```

Command-line flags can override part of the configuration:

```powershell
go run . -port 8090 -dir ./uploads -max-size 52428800 -title "LAN Image Transfer"
```

## API

- `POST /api/upload?path=<folder>`: upload one image file.
- `GET /api/files?path=<folder>`: list files and folders.
- `POST /api/create_folder`: create a folder.
- `DELETE /api/delete/{filename}?path=<folder>`: delete a file or folder.
- `GET /download/<path>`: download a file.

## Notes

This tool is intended for trusted local networks. If it is exposed beyond a LAN, add authentication and review upload/delete permissions first.

## License

MIT License. See `LICENSE`.
