# SIP Gateway

A WebRTC-to-SIP bridge that enables voice calls between browser clients and the PSTN through any SIP server.

This gateway is for demo purposes. 

## Architecture

```
Browser <-> WebRTC <-> [SIP Gateway] <-> SIP <-> SIP Server (Asterisk) <-> PSTN
                            |
                         Webhooks
                        (Chatwoot)
```

The gateway handles:

- **Inbound calls**: SIP INVITE from SIP trunk -> WebRTC offer to browser via webhook
- **Outbound calls**: Browser SDP offer -> SIP INVITE to SIP trunk
- **RTP bridging**: Bidirectional audio forwarding between SIP (RTP/PCMU) and WebRTC
- **Call recording**: Writes audio to WAV files (mu-law, 8kHz mono) and uploads finished recordings to Chatwoot storage
- **Webhook events**: Notifies Chatwoot of call lifecycle events
- **API endpoints**: For call management and signaling

## Prerequisites

- Go 1.23+
- Asterisk PBX (or compatible SIP server)
- Chatwoot instance with SIP channel support (Included in next commits)
- Or just Docker with the provided docker-compose file

## Configuration

All settings are configured via environment variables:

| Variable | Default | Description |
|---|---|---|
| `GATEWAY_SIP_PORT` | `5080` | UDP port for SIP signaling |
| `GATEWAY_HTTP_PORT` | `8080` | HTTP API port |
| `GATEWAY_PUBLIC_IP` | `192.168.1.6` | Public IP for SIP and HTTP server |
| `GATEWAY_STUN_SERVER` | `stun:stun.l.google.com:19302` | STUN server for ICE |
| `GATEWAY_RECORDING_DIR` | `/recordings` | Directory for call recordings |
| `GATEWAY_ICE_TCP_PORT` | `9565` | TCP port for ICE/WebRTC (TCP for more reliable connections fits more for local testing, but of course on production we'd use UDP for better performance) |
| `SIP_SERVER_HOST` | `127.0.0.1` | SIP server host |
| `SIP_SERVER_PORT` | `5060` | SIP server port |
| `CHATWOOT_WEBHOOK_URL` | `http://127.0.0.1:3000/webhooks/sip_gateway/events` | Chatwoot webhook endpoint to receive call events |
| `CHATWOOT_WEBHOOK_SECRET` | `test` | Shared secret for webhook auth (something simple for demo but in production we should use a more secure way to authenticate) |

## Running

```bash
# Docker Compose
docker-compose up -d
```

## HTTP API

All endpoints (except `/health`) require the `X-Gateway-Secret` header.
> Again the secret here is just a placeholder simulating a security layer


### `POST /calls/initiate`

Start an outbound call. Returns immediately with a `call_id` and `sdp_answer`; SIP setup happens asynchronously.

**Request:**
```json
{
  "to": "+15551234567",
  "from": "+15559876543",
  "sdp_offer": "v=0\r\n..."
}
```

**Response:**
```json
{"call_id": "uuid", "sdp_answer": "v=0\r\n..."}
```

### `POST /calls/{callID}/accept`

Accept an inbound call with a browser SDP answer.

**Request:**
```json
{
  "sdp_answer": "v=0\r\n..."
}
```

### `POST /calls/{callID}/reject`

Reject an inbound call (sends SIP 486 Busy Here).

### `POST /calls/{callID}/terminate`

End an active call (sends SIP BYE).

## Webhook Events

The gateway sends these events to Chatwoot:

| Event | Description | Key Fields |
|---|---|---|
| `call.incoming` | New inbound call | `call_id`, `from`, `to`, `sdp_offer`, `phone_number` |
| `call.ringing` | Remote party is ringing (outbound) | `call_id`, `phone_number` |
| `call.sdp_answer` | SDP answer ready (outbound) | `call_id`, `sdp_answer`, `phone_number` |
| `call.accepted` | Inbound call accepted | `call_id`, `phone_number` |
| `call.answered` | Outbound call answered | `call_id`, `phone_number` |
| `call.ended` | Call terminated | `call_id`, `reason`, `duration_seconds`, `phone_number` |

> **Note:** The `phone_number` field is used to identify the channel associated with the call, which is useful for routing. (Like number and DID in VoIP)

When a call has a recording, the gateway also uploads the WAV file to `POST /webhooks/sip_gateway/recordings` using the same `X-Gateway-Secret` header. Chatwoot stores it in Active Storage and updates the voice-call message `recording_url`.

### Termination Reasons

| Reason | Description |
|---|---|
| `agent-hangup` | Agent ended a connected call |
| `agent-canceled` | Agent canceled before answer |
| `remote-hangup` | Remote party ended a connected call |
| `remote-canceled` | Remote party canceled before answer |
| `no-answer` | No answer within timeout |
| `busy` | Remote party is busy (SIP 486) |
| `rejected` | Call was declined (SIP 603) |
| `not-found` | Destination not found (SIP 404) |

## Testing

```bash
go test ./...
```

## Project Structure

```
cmd/gateway/              Entry point
internal/
  api/                    HTTP API handlers
  bridge/                 RTP bridging (SIP <-> WebRTC)
  call/                   Call session and registry management
  config/                 Configuration loading from environment
  recording/              WAV file recording
  sip/                    SIP signaling server
    server.go             Server setup and initialization
    inbound.go            Inbound call handling (INVITE, ACK, accept/reject)
    outbound.go           Outbound call handling (SendInvite, answer wait)
    lifecycle.go          Call termination (BYE, CANCEL, cleanup)
    helpers.go            SDP generation/parsing, phone extraction
    reason.go             Hangup reason and status mapping
  webhook/                Webhook delivery client
  webrtc/                 WebRTC peer connection management
```
