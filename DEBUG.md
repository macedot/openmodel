# Debug Request/Response Files

When TRACE logging is enabled, openmodel saves request and response JSON files to `.openmodel_debug` directory for debugging purposes.

## Location

Debug files are saved in: `.openmodel_debug/`

## File Naming Convention

Files are named with the pattern: `{type}_{timestamp}_{request_id}_{short_uuid}.json`

- `type`: Either "request" or "response"
- `timestamp`: ISO format (YYYYMMDD-HHMMSS.mmm)
- `request_id`: The request ID from the User field or auto-generated UUID
- `short_uuid`: 8-character UUID suffix for uniqueness

Example:
```
request_20260306-154512.345_abc12345_a1b2c3d4.json
response_20260306-154517.890_abc12345_e5f6g7h8.json
```

## Enabling Debug Logging

Set log level to TRACE:

```bash
export OPENMODEL_LOG_LEVEL=trace
```

Or in config file:
```json
{
  "log_level": "trace"
}
```

## What Gets Saved

### Request Files
- Full JSON payload sent to provider
- Includes all messages (with assistant content sanitized)
- Includes all options and parameters
- **Note**: Assistant message content is always cleared in history to prevent llama.cpp prefill errors

### Response Files
- HTTP response status code
- Response body (up to 1MB for error responses)
- Full JSON response from provider

## Message Sanitization

**Important**: openmodel automatically clears content from all assistant messages in the conversation history before sending to llama.cpp. This is necessary because:

1. llama.cpp treats trailing assistant messages in history as "prefill" (prompt continuation)
2. This prefill behavior is incompatible with streaming and certain model configurations
3. Clearing assistant content prevents "Assistant response prefill is incompatible" errors

**When sanitization happens**: 
- Always for streaming requests (`StreamChat`)
- Always for non-streaming requests (`Chat`)
- Applies to all assistant messages, regardless of model configuration

## Use Cases

- Debugging "Assistant response prefill" errors
- Investigating request/response mismatches
- Understanding message structure before/after processing
- Verifying sanitization is being applied correctly

## Cleanup

To remove debug files:
```bash
rm -rf .openmodel_debug
```

Or add to `.gitignore`:
```
.openmodel_debug/
```
request_20260306-154512.345_abc12345_a1b2c3d4.json
response_20260306-154517.890_abc12345_e5f6g7h8.json
```

## Enabling Debug Logging

Set log level to TRACE:

```bash
export OPENMODEL_LOG_LEVEL=trace
```

Or in config file:
```json
{
  "log_level": "trace"
}
```

## What Gets Saved

### Request Files
- Full JSON payload sent to the provider
- Includes all messages (after sanitization if thinking is enabled)
- Includes all options and parameters

### Response Files
- HTTP response status code
- Response body (up to 1MB for error responses)
- Full JSON response from provider

## Use Cases

- Debugging "Assistant response prefill" errors
- Investigating request/response mismatches
- Understanding what sanitization is applied
- Verifying message structure before/after processing

## Cleanup

To remove debug files:
```bash
rm -rf .openmodel_debug
```

Or add to `.gitignore`:
```
.openmodel_debug/
```
