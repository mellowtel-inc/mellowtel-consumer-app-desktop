# Network Target Validation

## Issue

Desktop workers currently execute job-provided `url` and `method_endpoint` values without validating the scheme or resolved IP address. A job may therefore reach localhost, private LAN services, link-local/cloud-metadata addresses, or non-web schemes. Redirects and DNS rebinding may bypass hostname-only checks. Jobs may also supply `save_html_endpoint`, allowing results to be posted outside the normal Mellowtel endpoint.

## Risk

This is an SSRF-style boundary issue. The isolated browser profile protects the user's normal browser cookies and history, but it does not isolate the worker from the machine's network or filesystem.

## Required fix

- Permit only `http` and `https` job targets.
- Reject loopback, private, link-local, multicast, and unspecified IPv4/IPv6 addresses after DNS resolution.
- Revalidate every redirect and protect against DNS rebinding.
- Apply the policy to `url` and `method_endpoint`, including the simple-fetch path.
- Allowlist result destinations; do not trust arbitrary `save_html_endpoint` values.
- For stronger isolation, run browser workers without LAN routes and with restricted filesystem access.

## Acceptance criteria

Public HTTP(S) jobs continue working. Localhost, RFC1918/ULA, link-local, metadata, non-HTTP schemes, unsafe redirects, and unapproved result endpoints are rejected and logged without returning their content.
