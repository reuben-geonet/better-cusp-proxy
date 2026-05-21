# CUSP Local Proxy

`cusp-local-proxy` lets a modern browser on a Windows laptop reach old CUSP device web interfaces without weakening the browser globally.

The browser talks to a local proxy. The proxy talks to the CUSP device using legacy TLS 1.0 and weak cipher suites isolated to that one target connection.

## Why This Is Needed

Testing against a CUSP device showed direct modern TLS negotiation fails. Forcing legacy TLS works, but the device uses obsolete security:

- TLS 1.0 only.
- Weak cipher suites such as `DES-CBC3-SHA`, `IDEA-CBC-SHA`, and `RC4-SHA`.
- A self-signed, expired certificate from 2003.
- `md5WithRSAEncryption`.
- A 1024-bit RSA key.
- A certificate name that does not match the current device address.

Modern Firefox is right to reject that directly. This proxy keeps that compatibility downgrade out of Firefox.

## Usage

Run the proxy on the machine that can route to the desk device:

```sh
cusp-local-proxy 192.168.185.135
```

Then open:

```text
http://127.0.0.1:4444/
```

Use a different local port:

```sh
cusp-local-proxy -listen 127.0.0.1:8080 192.168.185.135
```

If you need the local side to be HTTPS:

```sh
cusp-local-proxy -local-tls 192.168.185.135
```

The default local HTTP mode is intentional. It avoids making users add a browser exception for a temporary localhost certificate.

## Build With Nix

Run locally on Linux or WSL:

```sh
nix run . -- 192.168.185.135
```

Build a Linux binary:

```sh
nix build
```

Build a Windows binary from Linux/WSL:

```sh
nix run .#build-windows
```

The Windows executable will be written to `dist/cusp-local-proxy.exe`.

## GitHub Actions Downloads

The repository includes a `Build` workflow that creates portable Windows executables without committing binaries to git:

- `cusp-local-proxy-windows-amd64.exe` for normal Intel/AMD Windows laptops.
- `cusp-local-proxy-windows-arm64.exe` for ARM64 Windows devices.
- `.sha256` checksum files for both.

For normal branch builds, download the executable from the workflow run's artifacts. For tagged GitHub Releases, the workflow also attaches the executables to the release assets.

## Notes

- This is a single-target proxy. Run one process per CUSP device.
- The target side intentionally disables certificate verification and forces TLS 1.0.
- Go does not support the IDEA TLS cipher. The tested device also accepted 3DES, which Go can use.
- If a device only accepts IDEA, this proxy will need an OpenSSL-backed transport instead of Go's standard TLS stack.
