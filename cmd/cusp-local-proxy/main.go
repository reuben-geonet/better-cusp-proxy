package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const defaultListen = "127.0.0.1:4444"

type config struct {
	listen       string
	localTLS     bool
	certFile     string
	keyFile      string
	certAge      time.Duration
	certOrg      string
	timeout      time.Duration
	shutdown     time.Duration
	hostHeader   string
	locationHost string
	verbose      bool
	target       string
}

func main() {
	cfg := parseFlags()

	targetAddr, err := normalizeTarget(cfg.target)
	if err != nil {
		log.Fatal(err)
	}
	if cfg.hostHeader == "" {
		cfg.hostHeader = targetAddr
	}

	targetURL := &url.URL{Scheme: "https", Host: targetAddr}
	proxy := newProxy(cfg, targetURL)

	server := &http.Server{
		Addr:              cfg.listen,
		Handler:           proxy,
		ReadHeaderTimeout: 15 * time.Second,
		TLSNextProto:      map[string]func(*http.Server, *tls.Conn, http.Handler){},
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errs := make(chan error, 1)
	go func() {
		scheme := "http"
		if cfg.localTLS {
			scheme = "https"
		}
		log.Printf("Starting local CUSP proxy at %s://%s", scheme, cfg.listen)
		log.Printf("Forwarding to legacy target https://%s", targetAddr)
		if cfg.localTLS {
			errs <- serveTLS(server, cfg)
			return
		}
		errs <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.shutdown)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("server shutdown: %v", err)
		}
	case err := <-errs:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}

	log.Print("server stopped")
}

func parseFlags() config {
	var cfg config

	flag.StringVar(&cfg.listen, "listen", defaultListen, "local listening address")
	flag.BoolVar(&cfg.localTLS, "local-tls", false, "serve HTTPS on localhost instead of HTTP")
	flag.StringVar(&cfg.certFile, "certfile", "", "optional local TLS certificate PEM file")
	flag.StringVar(&cfg.keyFile, "keyfile", "", "optional local TLS key PEM file")
	flag.DurationVar(&cfg.certAge, "age", 3*time.Hour, "generated local TLS certificate validity")
	flag.StringVar(&cfg.certOrg, "org", "GeoNet", "generated local TLS certificate organisation")
	flag.DurationVar(&cfg.timeout, "timeout", 30*time.Second, "target connection timeout")
	flag.DurationVar(&cfg.shutdown, "shutdown", 30*time.Second, "server shutdown timeout")
	flag.StringVar(&cfg.hostHeader, "host-header", "", "Host header to send to the target; defaults to host[:port]")
	flag.StringVar(&cfg.locationHost, "location-host", "", "public local host[:port] used when rewriting Location headers; defaults to listen address")
	flag.BoolVar(&cfg.verbose, "verbose", false, "log proxied requests")
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "Make a local browser proxy for old CUSP devices that only support legacy TLS.\n\n")
		fmt.Fprintf(out, "Usage:\n\n  %s [options] host[:port]\n\n", os.Args[0])
		fmt.Fprintf(out, "Open http://%s/ in the browser by default.\n\n", defaultListen)
		fmt.Fprintf(out, "Options:\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	cfg.target = flag.Arg(0)

	if cfg.certFile != "" || cfg.keyFile != "" {
		cfg.localTLS = true
	}
	if (cfg.certFile == "") != (cfg.keyFile == "") {
		log.Fatal("-certfile and -keyfile must be supplied together")
	}
	if cfg.locationHost == "" {
		cfg.locationHost = cfg.listen
	}

	return cfg
}

func normalizeTarget(target string) (string, error) {
	if target == "" {
		return "", errors.New("missing target")
	}
	if strings.Contains(target, "://") {
		u, err := url.Parse(target)
		if err != nil {
			return "", err
		}
		target = u.Host
	}
	host, port, err := net.SplitHostPort(target)
	if err == nil {
		if host == "" || port == "" {
			return "", fmt.Errorf("invalid target %q", target)
		}
		return net.JoinHostPort(host, port), nil
	}
	if strings.Contains(target, ":") {
		if ip := net.ParseIP(target); ip != nil {
			return net.JoinHostPort(target, "443"), nil
		}
		return "", fmt.Errorf("invalid target %q: %w", target, err)
	}
	return net.JoinHostPort(target, "443"), nil
}

func newProxy(cfg config, targetURL *url.URL) http.Handler {
	transport := &http.Transport{
		Proxy:               nil,
		DialTLSContext:      legacyTLSDialer(cfg.timeout),
		DisableCompression:  true,
		ForceAttemptHTTP2:   false,
		MaxIdleConns:        8,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     30 * time.Second,
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.Transport = transport
	proxy.ErrorLog = log.New(os.Stderr, "proxy: ", log.LstdFlags)
	proxy.Director = func(r *http.Request) {
		r.URL.Scheme = targetURL.Scheme
		r.URL.Host = targetURL.Host
		r.Host = cfg.hostHeader
		r.RequestURI = ""
		r.Header.Del("Proxy-Connection")
		r.Header.Del("Upgrade-Insecure-Requests")
		if cfg.verbose {
			log.Printf("%s %s -> %s", r.Method, r.URL.RequestURI(), targetURL.Host)
		}
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		rewriteLocation(resp, targetURL.Host, cfg)
		rewriteCookieDomains(resp)
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("proxy error for %s %s: %v", r.Method, r.URL.RequestURI(), err)
		http.Error(w, "CUSP proxy could not reach the target device: "+err.Error(), http.StatusBadGateway)
	}

	return proxy
}

func legacyTLSDialer(timeout time.Duration) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialer := &net.Dialer{Timeout: timeout}
		raw, err := dialer.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}

		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS10,
			MaxVersion:         tls.VersionTLS10,
			ServerName:         "",
			CipherSuites: []uint16{
				tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
				tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
				tls.TLS_RSA_WITH_RC4_128_SHA,
				tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
			},
		}

		conn := tls.Client(raw, tlsConfig)
		handshakeDone := make(chan error, 1)
		go func() {
			handshakeDone <- conn.Handshake()
		}()

		select {
		case <-ctx.Done():
			_ = raw.Close()
			return nil, ctx.Err()
		case err := <-handshakeDone:
			if err != nil {
				_ = raw.Close()
				return nil, err
			}
			return conn, nil
		}
	}
}

func rewriteLocation(resp *http.Response, targetHost string, cfg config) {
	location := resp.Header.Get("Location")
	if location == "" {
		return
	}

	u, err := url.Parse(location)
	if err != nil || !u.IsAbs() {
		return
	}
	if !sameHost(u.Host, targetHost) && cfg.hostHeader != "" && !sameHost(u.Host, cfg.hostHeader) {
		return
	}

	if cfg.localTLS {
		u.Scheme = "https"
	} else {
		u.Scheme = "http"
	}
	u.Host = cfg.locationHost
	resp.Header.Set("Location", u.String())
}

func sameHost(a, b string) bool {
	ah, ap := splitHostDefault(a)
	bh, bp := splitHostDefault(b)
	return strings.EqualFold(ah, bh) && ap == bp
}

func splitHostDefault(addr string) (string, string) {
	host, port, err := net.SplitHostPort(addr)
	if err == nil {
		return host, port
	}
	return addr, "443"
}

func rewriteCookieDomains(resp *http.Response) {
	values := resp.Header.Values("Set-Cookie")
	if len(values) == 0 {
		return
	}

	resp.Header.Del("Set-Cookie")
	for _, value := range values {
		parts := strings.Split(value, ";")
		out := parts[:0]
		for _, part := range parts {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(part)), "domain=") {
				continue
			}
			out = append(out, part)
		}
		resp.Header.Add("Set-Cookie", strings.Join(out, ";"))
	}
}

func serveTLS(server *http.Server, cfg config) error {
	cert, err := localCertificate(cfg)
	if err != nil {
		return err
	}
	server.TLSConfig = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
	}

	ln, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return err
	}
	return server.Serve(tls.NewListener(ln, server.TLSConfig))
}

func localCertificate(cfg config) (tls.Certificate, error) {
	if cfg.certFile != "" && cfg.keyFile != "" {
		return tls.LoadX509KeyPair(cfg.certFile, cfg.keyFile)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return tls.Certificate{}, err
	}

	now := time.Now()
	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{cfg.certOrg},
			CommonName:   "localhost",
		},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(cfg.certAge),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses: []net.IP{
			net.ParseIP("127.0.0.1"),
			net.ParseIP("::1"),
		},
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	return tls.X509KeyPair(certPEM, keyPEM)
}
