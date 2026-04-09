# How to Run

## Prerequisites

- Docker and Docker Compose
- A SIP softphone for testing (e.g., MicroSIP, or Linphone)

---

## Step 1: Start the Telephony Stack

The telephony stack (Asterisk + Go SIP Gateway) runs in its own Docker Compose file.

```bash
cd telephony
docker compose up -d
```

This starts:
- **Asterisk** (PBX) at `192.168.107.2` — handles SIP registration and PSTN simulation
- **Go SIP Gateway** (B2BUA) at `192.168.107.3` — bridges WebRTC to SIP


Verify both are running:
```bash
docker compose ps
docker compose logs gateway   # should show "SIP server listening on :5080"
```

---

## Step 2: Start Chatwoot

From the project root:

```bash
cd ..   # back to chatwoot root
# copy the .env file from the telephony directory
cp .env.example .env

# first build the base image so that you avoid docker trying to pull it
docker compose build base

# start the containers
docker compose up -d

# create the database
docker compose run --rm rails sh -c 'RAILS_ENV=development bundle exec rails db:create'

# prepare the database
docker compose run --rm rails sh -c 'RAILS_ENV=development bundle exec rails db:chatwoot_prepare'
```

This starts:
- **Rails** (API + ActionCable) at `http://localhost:3000`
- **Sidekiq** (background jobs — webhook processing, call timeouts)
- **Vite** (frontend dev server)
- **PostgreSQL**, **Redis**, **Mailhog**

The Rails and Sidekiq containers are configured to reach the gateway at `gateway.local` (mapped to `192.168.107.3` via `extra_hosts`) over the shared `telephony_sip_gateway` Docker network.

Wait for Chatwoot to be ready:
```bash
docker compose logs -f rails   # wait for "Listening on http://0.0.0.0:3000"
```

---

## Step 3: Configure the SIP Inbox in Chatwoot

1. Open `http://localhost:3000` and log in with the seeded user john@acme.inc:Password1!
> wait sometime so that the chatwoot FE compiles as we are running it in dev mode
2. Go to **Settings > Inboxes > Add Inbox**
3. Select **SIP** as the channel type
4. Fill in:
   - **Phone Number**: `+966555555555` (must match a tenant in `telephony/gateway/tenants.yaml`)
   - **API Key**: `tenant-key-2` (the corresponding key from `tenants.yaml`)
5. Save and assign agents to the inbox

---

## Step 4: Register a SIP Softphone (Customer Simulation)

Configure your softphone to register with Asterisk:

| Setting    | Value                |
|------------|----------------------|
| Server     | `127.0.0.1:5060`    |
| Username   | `1001`               |
| Password   | `test1001`           |
| Transport  | UDP                  |

Once registered, you can simulate a customer calling your business number.

---

## Step 5: Test Inbound Call

1. From the softphone, dial any number (e.g., `+966555555555`)
2. Asterisk routes the call to the Gateway
3. Gateway sends a webhook to Chatwoot → creates a conversation → notifies agents via ActionCable
4. In the Chatwoot browser, you should see the incoming call notification
5. Click **Accept** — WebRTC audio connects through the Gateway to the softphone
6. Hang up — the conversation shows call duration and status

---

## Step 6: Test Outbound Call

1. In Chatwoot, open a conversation with a contact that has a phone number
2. Click the **Call** button
3. The browser creates a WebRTC offer → Chatwoot sends it to the Gateway → Gateway sends SIP INVITE to Asterisk → Asterisk rings the softphone
4. Answer on the softphone — audio flows
5. Hang up — call summary appears in the conversation

---

## Environment Variables

### Chatwoot (`.env`)
```
SIP_GATEWAY_URL=http://gateway.local    # Gateway HTTP API (used by Rails/Sidekiq)
SIP_GATEWAY_SECRET=test                 # Shared secret for webhook verification
```

### Gateway (`telephony/docker-compose.yml`)
Key variables are pre-configured. See the file for the full list.

### Tenants (`telephony/gateway/tenants.yaml`)
Maps phone numbers to API keys. Each Chatwoot SIP inbox must have a matching entry here.

---

## Network Architecture

```
chatwoot (default network)          telephony (sip_gateway network)
+--------+  +--------+  +-------+     +----------+  +-----------+
| Rails  |  |Sidekiq |  | Vite  |     | Gateway  |  | Asterisk  |
| :3000  |  |        |  | :3036 |     | .107.3   |  | .107.2    |
+---+----+  +---+----+  +-------+     +----+-----+  +-----+-----+
    |            |                          |              |
    +-------+----+----- telephony_sip_gateway (shared) ----+
            |
    +-------+--------+
    | PostgreSQL Redis|
    | Mailhog         |
    +-----------------+
```

The `telephony_sip_gateway` network is created by the telephony stack and declared as `external` in Chatwoot's `docker-compose.yaml`, allowing Rails and Sidekiq to communicate with the Gateway.

---

## Troubleshooting

- **Gateway not reachable from Rails**: Make sure the telephony stack is started *before* Chatwoot, so the `telephony_sip_gateway` network exists.
- **No incoming call notification**: Check that the phone number in the SIP inbox matches `tenants.yaml` and that Sidekiq is running (`docker compose logs sidekiq`).
- **WebRTC audio not working**: Ensure ports `9050` (ICE TCP) is accessible..
- **Softphone can't register**: Verify Asterisk is running and port `5060/udp` is exposed. Check `docker compose logs asterisk`.
