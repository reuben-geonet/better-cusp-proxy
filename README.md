# CUSP Local Proxy

`cusp-local-proxy` lets a modern browser reach a CUSP device's web interface.

The browser talks to a local proxy. The proxy talks to the CUSP device using legacy TLS 1.0 and weak cipher suites isolated to that one target connection.

## Why This Is Needed

CUSP devices use now unsupported security protocols.

- TLS 1.0 only.
- Weak cipher suites such as `DES-CBC3-SHA`, `IDEA-CBC-SHA`, and `RC4-SHA`.
- A self-signed, expired certificate from 2003.
- `md5WithRSAEncryption`.
- A 1024-bit RSA key.
- A certificate name that does not match the current device address.

## Usage

Run the proxy on the machine:

```sh
cusp-local-proxy 192.168.0.100
```

Then open:

```text
http://127.0.0.1:4444/
```

Use a different local port:

```sh
cusp-local-proxy -listen 127.0.0.1:8080 192.168.0.100
```

If you need the local side to be HTTPS:

```sh
cusp-local-proxy -local-tls 192.168.0.100
```

The default local HTTP mode is intentional. It avoids making users add a browser exception for a temporary localhost certificate.

## Build With Nix

Run locally on Linux or WSL:

```sh
nix run . -- 192.168.0.100
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
