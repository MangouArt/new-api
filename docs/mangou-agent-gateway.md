# Mangou Agent Gateway Plan

## Goal

Use NewAPI as the only gateway and billing system for Mangou agent traffic. Mangou should not keep a separate billing service. NewAPI owns users, tokens, quota, channels, task lifecycle, logs, recharge, and provider credentials. Mangou adds an agent-friendly API layer and provider-specific task adapters.

## Scope

- Image and video generation both use NewAPI asynchronous `Task` records.
- Provider selection is managed by provider group, not by splitting every upstream model into a separate billing identity.
- Pricing is calculated from request parameters using provider-specific pricing config.
- Agent registration requires email verification and reuses NewAPI's verification code primitives.
- Agent clients only need an email during registration, then store the returned token as `BILLING_TOKEN`.
- Demo recharge uses NewAPI `top_ups` rows and a scan URL: the agent requests a QR URL, the user opens/scans it, and NewAPI marks the payment paid and credits the account.

## Provider Group Model

Agent requests keep the upstream-facing fields:

```json
{
  "type": "image",
  "provider": "bltai",
  "model": "nano-banana-2",
  "prompt": "A mango robot painting a storyboard, no text.",
  "params": {
    "image_size": "1K",
    "aspect_ratio": "16:9"
  }
}
```

NewAPI maps them internally:

```text
UsingGroup       = NewAPI token/user group, e.g. auto or default
Platform         = bltai
Action           = image.generate
OriginModelName  = mangou-image
UpstreamModelName = nano-banana-2
```

Provider routing must be configured in NewAPI native tables before agents submit tasks:

- `channels`: provider key, base_url, enabled status, supported models, enabled groups
- `abilities`: group/model/channel routing
- `models`: model metadata shown in `/console/models`
- `mangou_provider_pricings`: provider/type pricing rules

Use the root-protected sync endpoint after configuring provider bootstrap env vars or channel keys:

```text
POST /api/mangou/providers/sync
```

This endpoint upserts Mangou channel/model/ability rows. Runtime task submission only reads configured NewAPI rows; it does not create channels lazily. If a provider/model/group is missing, `/v1/agent/tasks` returns an actionable `success: false` error naming the missing provider/model/group.

The token/user group chooses channel routing and group-level quota multiplier. The upstream `model` remains a provider payload parameter and is also registered in NewAPI's native model/ability tables so the admin console can show it.

## Async Task Lifecycle

Endpoints:

```text
POST /v1/agent/tasks
GET  /v1/agent/tasks/:task_id
GET  /v1/agent/tasks
```

Submit behavior:

1. Authenticate with NewAPI token auth.
2. Validate `type`, `provider`, `model`, `prompt`, and `params`.
3. Resolve pricing config by `provider` and `type`.
4. Calculate estimated quota from base quota and parameter multipliers.
5. Pre-consume user/token quota.
6. Insert a `model.Task` with status `SUBMITTED`.
7. Store canonical request, pricing snapshot, and upstream task id/result fields in `Task.Data` and `Task.PrivateData`.
8. Return public `task_id` and polling URL.

Polling behavior:

1. Load task by authenticated user and `task_id`.
2. Return normalized status, progress, result URL, provider, model, type, and quota.
3. Provider adapters later update the same task row from upstream polling.

Failure behavior:

- If upstream submission fails before task persistence, refund pre-consumed quota.
- If polling reaches terminal failure, reuse task billing refund logic.

## Pricing Config

Pricing must be configurable because upstream provider pricing changes. Do not hard-code provider docs into request logic, and do not keep pricing in env vars.

Single source of truth:

```text
mangou_provider_pricings table
```

Table columns:

```text
provider
task_type
base_quota
multipliers_json
enabled
created_at
updated_at
```

`multipliers_json` shape:

```json
{
  "image_size": {
    "1K": 1,
    "2K": 2
  },
  "quality": {
    "standard": 1,
    "hd": 1.5
  }
}
```

Example row:

```text
provider=bltai
task_type=image
base_quota=100
multipliers_json={"image_size":{"1K":1,"2K":2},"quality":{"standard":1,"hd":1.5}}
enabled=true
```

## Agent Registration

Endpoints:

```text
POST /v1/agents/register/email-code
POST /v1/agents/register
```

Email code request:

```json
{
  "email": "user@example.com"
}
```

Registration request:

```json
{
  "email": "user@example.com",
  "verification_code": "123456",
  "agent_id": "hermes-mangou"
}
```

Server behavior:

1. Reuse NewAPI verification primitives:
   - `common.GenerateVerificationCode`
   - `common.RegisterVerificationCodeWithKey`
   - `common.VerifyCodeWithKey`
   - `common.DeleteKey`
   - `common.SendEmail`
2. If email does not exist, create a common user with a generated username.
3. If email exists, reuse that user.
4. Ensure the agent user and token use the `auto` group so provider routing can select any enabled provider group.
5. Create or reuse a token named for the agent.
6. Return the full `billing_token` so headless agents can persist and use it without a dashboard handoff.

## First Implementation Milestone

- Add tests first for:
  - verified email registration creates a user and token;
  - invalid verification code is rejected;
  - image task submit stores a provider-group task and pre-consumes parameter-priced quota;
  - unsupported provider/type pricing config is rejected.
- Implement local async task creation and pricing calculation.
- Add routes.
- Keep upstream HTTP submission as the next milestone, implemented through provider-specific `TaskAdaptor`s.

## Agent Balance And Recharge

Protected endpoints use the same `Authorization: Bearer ${BILLING_TOKEN}` header as task submission:

```text
GET  /v1/agent/balance
GET  /v1/agent/credits
POST /v1/agent/recharge-qr
POST /v1/agent/recharge
POST /v1/agent/topup
POST /v1/agent/payment
```

Legacy-compatible aliases are also available:

```text
POST /v1/agents/recharge-qr
POST /v1/agents/recharge
```

Recharge request:

```json
{
  "agent_id": "mangou-agent",
  "tier": "gems_100",
  "amount": 100,
  "return_url": "https://example.com/after-payment"
}
```

`amount` is optional when `tier` is one of:

```text
gems_10
gems_100
gems_1000
```

Response:

```json
{
  "payment_id": "pay_...",
  "payment_status": "pending",
  "amount": 100,
  "currency": "credits",
  "provider": "creem",
  "demo": false,
  "qr_url": "https://mangou-newapi.zeabur.app/v1/payments/pay_.../qr.svg",
  "qr_png_url": "https://mangou-newapi.zeabur.app/v1/payments/pay_.../qr.png",
  "payment_url": "https://creem.io/test/checkout/prod_.../ch_..."
}
```

Public scan endpoints:

```text
GET /v1/payments/{payment_id}/qr.svg
GET /v1/payments/{payment_id}/qr.png
GET /v1/payments/demo-scan/{payment_id}
```

The scan endpoint is idempotent. The first successful scan marks the `top_ups` row `success` and adds `amount` credits to the NewAPI user quota. Repeated scans return the success page without adding quota again.

When a task submit fails with an insufficient-balance message such as `quota is not enough`, `user quota is not enough`, or `余额不足`, the agent should call `POST /v1/agent/recharge-qr`, send `qr_png_url` as a chat image attachment, include `payment_url` as a fallback link, ask the user to scan/recharge, poll `/v1/agent/balance` every 10-20 seconds, and retry the original task once after the balance increases.

## Provider Runtime

The first provider runtime uses direct HTTP task submission from the agent task controller:

- `evolink`: unified async API, `POST /v1/images/generations`, `POST /v1/videos/generations`, `GET /v1/tasks/{task_id}`.
- `bltai`: treated as the same unified async API. `BLTAI_BASE_URL` may already include `/v1`.
- `kie`: Runway video API, `POST /api/v1/runway/generate`, `GET /api/v1/runway/record-detail?taskId=...`.

Required env vars:

```text
EVOLINK_API_KEY
BLTAI_API_KEY
BLTAI_BASE_URL
KIE_API_KEY
```

Optional env vars:

```text
EVOLINK_BASE_URL=https://api.evolink.ai
KIE_BASE_URL=https://api.kie.ai
```

If a provider key is missing, NewAPI still creates the local async task and pre-consumes quota, but it does not submit to the upstream runtime. This keeps local development and pricing tests independent from external providers.
