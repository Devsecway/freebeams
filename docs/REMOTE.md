# Remote control (PLAN v0.2 item 2)

A phone or tablet on the church Wi-Fi drives FreeBeams from a web page: the schedule, the Preview and Live slide
lists, Next / Prev, Black / Clear / Logo, going live with a schedule item, and adding a library song to the
schedule. Nothing is installed on the phone. The page is plain HTML, CSS and JavaScript embedded in the binary
(`internal/remote/web`), with no external requests, because church Wi-Fi is often offline.

## Using it

1. Turn it on: toolbar **Remote ▾ › Remote Control On**, or **Options › Remote** (switch and port, default 8765).
   The setting persists (`remote.enabled`, `remote.port`). The server starts with the app when it is on.
2. **Remote ▾ › Show Pairing Code…** opens the pairing dialog. It shows a QR code for the address
   (`http://<LAN address>:<port>/`), the address as text, and a six-digit pairing code.
3. On the phone, scan the code or type the address, enter the pairing code and a device name, and tap Pair.
4. The phone appears under **Waiting for approval** in the dialog, with its kind and IP address. **Approve** it,
   or **Deny** it. Approved phones are listed under **Paired devices** (Connected / Seen 5 min ago) and stay paired
   across restarts until you **Remove** them there, or the phone uses "Forget this device".

## Security model

The remote listens on every interface, so the server assumes that anyone on the network may try to use it.

- **Pairing needs the code and the operator.** A phone that knows the code can only *ask*. The operator approves
  it by name in FreeBeams. Each code pairs one device: approving one makes a new code. **New Code** replaces it at
  any time.
- **Guessing is rate-limited.** 5 wrong codes per address per 5 minutes, then that address waits. After 20 wrong
  codes from everyone in 5 minutes, the code changes by itself. At most 5 requests wait at once, one per address,
  and a request expires after 2 minutes.
- **Per-device tokens.** Approval creates a 256-bit random token. The phone collects it once, with the request's
  own secret (only the phone that asked has it), as an `HttpOnly`, `SameSite=Strict` cookie that lasts 400 days.
  FreeBeams stores only its SHA-256 hash (`remote.devices` in the library database), so a copy of the database
  lets nobody in. Every API call and the WebSocket need the token, compared in constant time.
- **Revocation is immediate.** Removing a device drops its hash and closes its open sockets. The phone is told it
  was removed, and its token no longer works (401).
- **Local network only.** Requests from non-local addresses are refused (loopback, RFC 1918, `fc00::/7`,
  link-local and CGNAT `100.64/10` only). A `Host` header must be an IP literal, `localhost` or a `.local` name,
  which blocks DNS rebinding. POSTs must be JSON (a cross-site form cannot send it without a CORS preflight, which
  is never answered) and, like the WebSocket upgrade, must come from the page's own `Origin`. Responses carry
  `nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer` and a strict CSP.
- **A whitelist of actions.** A phone reaches only the actions in `internal/remote/actions.go`. Each is rebuilt
  from a few typed fields and mapped to one app action: `next`, `prev`, `first`, `last`, `black`/`clear`/`logo`
  (`{on}` sets the mode rather than toggling it, so two phones agree), `go-live {slide}`, `preview-item {index}`,
  `live-item {index}`, `live-slide {index}`, `add-song {id}`. Device permissions (view, control, schedule) gate
  them. Approval grants all three today. The loopback control server (`internal/control`, its own bearer key) is
  separate and never exposed. Files, settings, quitting and key events cannot be reached from the network.
- **No TLS.** The LAN is trusted for confidentiality. The token travels in clear on the Wi-Fi, so an attacker who
  can sniff the network can replay it until the device is removed. A self-signed certificate would make every
  phone show a warning page.

PLAN.md proposed reusing the control server's bearer key in the QR code. That was not done: the key would be one
shared secret for every phone, it could not be revoked per device, and it would also open the test API.

## How it fits the app

- `internal/remote` is pure Go (no Godot). It holds the HTTP server, pairing, devices, the WebSocket hub, the
  phone's state document (`remote.BuildState`) and the QR encoder (`rsc.io/qr`). `remote_test.go` covers pairing,
  expiry, the rate limiter, token auth, the guards, the origin checks, revocation and the WebSocket.
- Phone actions become `control.Command`s on `remote.Server.Commands()`. `App.Process` drains them like the control
  API's, through `handleCommand`, so a tap takes the operator's own event path. The reply is sent after the render
  pass.
- `App.render` publishes the phone's state on every pass (`renderRemote`). `Server.Publish` sends a WebSocket push
  only when the JSON changed. A slow phone gets the newest state, never a backlog.
- `remote.Server.Changed()` (pairing requests, approvals, connections) refreshes the status that `/state.remote`
  and the pairing dialog show, and persists the devices when they changed. The status is also re-read once a
  second while the server runs, so requests expire and "Seen … ago" moves on.
- The pairing dialog (`ui.RemoteDialog`) and the Options page are dumb views. The QR code is an RGBA image with one
  pixel per module, drawn with nearest filtering.

## Automation

`/state.remote`: `enabled, configuredPort, error, dialog ("pairing" while the dialog is up), running, port,
addresses, url, code, pending [{id, name, kind, address, created}], devices [{id, name, kind, online, created,
lastSeen}], clients`.

Control actions: `remote {on, port}`, `remote-dialog {on}`, `remote-approve {device | name}`,
`remote-deny {device | name}`, `remote-revoke {device | name}`, `remote-new-code`, the settings keys
`remote.enabled` / `remote.port` in `set-setting`, `options {page: "remote"}`, and `screenshot?window=remote`. The
phone-side actions `remote-live-item {index}` and `remote-add-song {id}` are control actions too.

`test/remoteclient` is a fake phone (`pair`, `wait`, `ws -do ACTION…`, `watch`) that speaks the page's own API.
`test/smoke.sh` uses it in the `# ---- remote control` block.
