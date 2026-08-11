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

## Server work required before merging

1. Add the authenticated Cloudflare route `POST /api/devices/register`.
2. Add an AWS device service (Lambda + DynamoDB) that owns the immutable mapping
   from Cognito `sub` to stable device ID.
3. Issue signed, short-lived device proofs containing user ID, device ID,
   audience, issued-at, and expiry claims. Rotate the signing key in AWS Secrets
   Manager and expose only verification material to the node gateway.
4. Make `ws.mellow.tel` and `api.mellow.tel/approval` reject a missing, expired,
   or mismatched proof before accepting work from the node.
5. Have the trusted Mellowtel result/revenue service—not the desktop—emit
   idempotent usage and earnings events into the AWS rewards ledger.

## Dashboard and payout endpoints still needed

- `GET /api/devices`: signed-in user's devices, last seen, app version, state.
- `GET /api/rewards/summary`: verified, pending, promotional, and withdrawable
  balances. Never calculate money from client-reported job counts.
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
