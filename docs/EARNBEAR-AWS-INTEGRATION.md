# Earnbear desktop ↔ AWS integration contract

The desktop must not call the affiliate Lambda directly and must never contain
an AWS HMAC secret. Its only authenticated control-plane endpoint is
`https://earnbear.app`, which validates the existing HttpOnly Cognito session
and signs any AWS request server-side.

## Implemented on this branch

Before sharing starts, the desktop calls:

```http
POST /api/devices/register
Cookie: earnbear_access=...; earnbear_refresh=...
Content-Type: application/json

{
  "deviceId": "mllwtl_consumer_abc123",
  "appVersion": "0.1.0-mvp",
  "protocolVersion": "700.0.29",
  "platform": "darwin",
  "integration": "consumer"
}
```

The server must derive the user ID from the validated Cognito session, upsert
the device-to-user link, and return a short-lived opaque credential:

```json
{
  "success": true,
  "deviceToken": "opaque-short-lived-proof",
  "linkedAt": "2026-08-11T10:00:00Z"
}
```

The desktop holds that token in memory and sends it as
`X-Earnbear-Device-Token` on both the Mellowtel WebSocket handshake and approval
checks. Do not place the proof in URL query parameters, where proxies and logs
commonly retain it.

## Companion server work implemented locally

The companion Earnbear website/backend implementation now provides:

1. Authenticated Cloudflare route `POST /api/devices/register`.
2. AWS Lambda + DynamoDB device registry with an immutable device-to-Cognito
   account mapping.
3. Random opaque device credentials whose SHA-256 digests are stored in AWS.
   Credentials expire after 30 days and are rotated whenever sharing starts.
4. Service-only `POST /v1/devices/verify` for gateway-side credential checks.
5. An idempotent, micro-dollar rewards ledger ingress for the trusted Mellowtel
   result/revenue service.

These pieces are tested locally but must be deployed before using this branch.

## Server work still required before release

1. Make `ws.mellow.tel` and `api.mellow.tel/approval` call the AWS verification
   endpoint and reject a missing, expired, or mismatched credential before
   accepting work from the node.
2. Have the trusted Mellowtel result/revenue service—not the desktop—emit
   idempotent verified earnings events into the AWS rewards ledger.
3. Decide whether the gateway should cache successful verification briefly to
   avoid one Lambda call per approval or socket reconnect.

## Dashboard endpoints implemented locally

- `GET /api/devices`: signed-in user's devices, last seen, app version, state.
- `GET /api/rewards/summary`: verified, pending, promotional, and withdrawable
  balances. Never calculate money from client-reported job counts.

The account dashboard reads both through `GET /api/account/overview`.

## Payout endpoints still needed

- `POST /api/payouts/onboarding-session`: creates the provider-hosted recipient
  onboarding URL/widget session.
- `POST /api/payouts/withdraw`: enforces method-specific minimums, the first
  payout hold, velocity limits, identity state, and idempotency.
- Provider webhooks: authoritative paid/failed/reversed state transitions.

Recommended reward states are `pending`, `verified`, `withdrawable`, `reserved`,
`paid`, and `reversed`. Promotional credits must remain separate from verified
bandwidth earnings so a $2 welcome credit cannot unlock its own cashout.

## Trust boundary

The desktop may report liveness and display server balances, but it is never an
authority for earnings, fraud clearance, payout eligibility, or payout status.
Bank details and wallet onboarding data should go directly to the chosen payout
provider; Earnbear stores provider recipient and transfer IDs only.
