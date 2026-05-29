# easy-up

easy-up is a lightweight LAN image transfer tool built with Go. It provides a browser-based interface for uploading, browsing, downloading, and deleting image files on a local network.

## Features

- Upload images, office documents, and common code/text files by drag and drop or file picker.
- Browse uploaded files and folders.
- Create folders.
- Download files from the browser.
- Delete files or folders when enabled.
- Sign in with a configured username and password.
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
  "max_file_size": 5368709120,
  "allow_deletes": true,
  "title": "局域网图片传输工具",
  "users": [
    {
      "username": "admin",
      "password": "admin123"
    }
  ],
  "session_secret": "",
  "session_ttl_hours": 12
}
```

`users` is the login whitelist. Passwords are stored in plain text by design for this small LAN tool, so do not publish a real `config.json` with private credentials. If `session_secret` is empty, the server generates a temporary signing key and users must log in again after restart.

Command-line flags can override part of the configuration:

```powershell
go run . -port 8090 -dir ./uploads -max-size 5368709120 -title "LAN Image Transfer"
```

## API

- `POST /api/login`: sign in with username and password.
- `POST /api/logout`: clear the current login session.
- `GET /api/session`: check current login state.
- `POST /api/upload?path=<folder>`: upload one image file.
- `GET /api/files?path=<folder>`: list files and folders.
- `POST /api/create_folder`: create a folder.
- `DELETE /api/delete/{filename}?path=<folder>`: delete a file or folder.
- `GET /download/<path>`: download a file.

All file APIs require login. Uploads are limited by `max_file_size`; duplicate filenames are saved as `name-1.ext`, `name-2.ext`, and so on. Supported uploads include common image formats, PDF, Word, Excel, PowerPoint, Markdown, plain text, CSV, JSON/XML/YAML, mainstream programming source files, and common archives such as ZIP, RAR, 7Z, TAR, GZ, TGZ, BZ2, and XZ. SVG uploads are disabled. Folder deletion only removes empty folders.

## Notes

This tool is intended for trusted local networks. If it is exposed beyond a LAN, use stronger password storage, HTTPS, and stricter access controls.

## License

MIT License. See `LICENSE`.
