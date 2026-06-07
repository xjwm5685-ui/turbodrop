﻿﻿﻿# TurboDrop API

TurboDrop exposes a small local HTTP API for the Web UI and automation scripts.

Base URL:

```text
http://localhost:48080/api/v1
```

## Endpoints

### `GET /info`

Returns current device information.

Example response:

```json
{
  "name": "TurboDrop Device",
  "ip": "192.168.1.10",
  "platform": "windows",
  "quic_port": 9001
}
```

### `POST /receive`

Starts receiver mode and returns a generated PIN.

Request body:

```json
{
  "device_name": "My Device"
}
```

Response:

```json
{
  "success": true,
  "pin": "582347",
  "message": "接收端已启动"
}
```

### `POST /upload`

Accepts a browser-selected file as multipart form data and stores it in `./uploads`.

Notes:
- Requests larger than the server-side upload limit are rejected.
- Uploaded files are treated as temporary working files.

Form field:

```text
file=<binary>
```

Response:

```json
{
  "success": true,
  "message": "文件上传成功",
  "filepath": "./uploads/upload-123456.txt",
  "original_name": "example.txt",
  "size": 1234
}
```

### `POST /send`

Starts send mode using either uploaded temporary files or existing local file paths.

Request body:

```json
{
  "pin": "582347",
  "files": [
    {
      "filepath": "./uploads/upload-123456.txt",
      "filename": "example.txt",
      "size": 1234
    },
    {
      "filepath": "./uploads/upload-789012.txt",
      "filename": "notes.pdf",
      "size": 4567
    }
  ]
}
```

Backward-compatible single-file payloads using `filepath` + `filename` are still accepted.

Response:

```json
{
  "success": true,
  "message": "队列已启动，共 2 个文件"
}
```

### `GET /history`

Returns persisted transfer history.

Response:

```json
{
  "success": true,
  "items": [
    {
      "id": "1749280000000000000",
      "filename": "example.txt",
      "status": "success",
      "message": "文件传输成功",
      "size": 1234,
      "device_name": "TurboDrop Device",
      "device_ip": "192.168.1.10",
      "started_at": "2026-06-07T12:00:00Z",
      "completed_at": "2026-06-07T12:00:02Z"
    }
  ]
}
```

### `GET /config`

Returns the persisted application configuration used by the settings panel.

Response:

```json
{
  "success": true,
  "config": {
    "web_host": "localhost",
    "web_port": 48080,
    "device_name": "TurboDrop Device",
    "quic_port": 9001,
    "max_concurrent_streams": 16,
    "chunk_size_mb": 4,
    "save_dir": "./received_files"
  }
}
```

### `PUT /config`

Saves application settings.

Request body:

```json
{
  "web_host": "localhost",
  "web_port": 48080,
  "device_name": "TurboDrop Device",
  "quic_port": 9001,
  "max_concurrent_streams": 16,
  "chunk_size_mb": 4,
  "save_dir": "./received_files"
}
```

Response:

```json
{
  "success": true,
  "message": "配置已保存",
  "config": {
    "web_host": "localhost",
    "web_port": 48080,
    "device_name": "TurboDrop Device",
    "quic_port": 9001,
    "max_concurrent_streams": 16,
    "chunk_size_mb": 4,
    "save_dir": "./received_files"
  },
  "requires_restart": false
}
```

### `POST /config/select-save-dir`

Opens a native folder picker on the local machine and returns the selected directory path.

Request body:

```json
{
  "current_path": "./received_files"
}
```

Response when a folder is selected:

```json
{
  "success": true,
  "path": "D:\\TurboDrop\\Received",
  "message": "已选择默认保存位置"
}
```

Response when the picker is canceled:

```json
{
  "success": true,
  "canceled": true,
  "message": "已取消选择"
}
```

### `GET /ws` / `WS /ws`

WebSocket endpoint used by the dashboard for real-time events.

Event types:

- `pin_generated`
- `device_found`
- `transfer_start`
- `progress`
- `transfer_done`
- `queue_status`
- `log`
- `error`

## Notes

- The API is intended for local network / local host use.
- HTTP / WebSocket origins are restricted to local loopback origins such as `localhost`, `127.0.0.1`, and `::1`.
- When a send request uses a managed upload file, TurboDrop removes it after the transfer attempt finishes.
- `web_host` / `web_port` changes are persisted immediately but require an application restart to change the active listener.
- `device_name`, `quic_port`, `max_concurrent_streams`, and `chunk_size_mb` affect subsequent new tasks after saving.
- `save_dir` controls where received files are stored by default.
- The settings page can call `POST /config/select-save-dir` to open a native folder picker instead of requiring manual path entry.
