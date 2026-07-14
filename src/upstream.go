// Package main implements the Minewire proxy server.
// This file implements an optional upstream SOCKS5 proxy dialer, used so the
// server can route outbound (target-side) connections through something like
// a local WARP SOCKS proxy (e.g. 127.0.0.1:4000) instead of dialing directly.
//
// This is a minimal, dependency-free SOCKS5 CONNECT client - it deliberately
// avoids pulling in golang.org/x/net just for this, since that dependency
// can't be resolved through this environment's restricted module proxy setup.
// It supports the "no auth" and "username/password" SOCKS5 auth methods,
// which covers WARP's local socks proxy as well as most third-party SOCKS5
// upstreams.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"time"
)

// socks5 reply codes we care about (RFC 1928 section 6)
const (
	socks5Version    = 0x05
	socks5CmdConnect = 0x01

	socks5AuthNone      = 0x00
	socks5AuthUserPass  = 0x02
	socks5AuthNoAccept  = 0xFF
	socks5UserPassOK    = 0x00
	socks5AtypIPv4      = 0x01
	socks5AtypDomain    = 0x03
	socks5AtypIPv6      = 0x04
	socks5ReplySucceded = 0x00
)

// dialUpstream connects to dest ("host:port"), optionally routing the
// connection through the configured upstream SOCKS5 proxy. If no upstream
// proxy is configured, it falls back to a direct dial - matching the
// server's previous (pre-upstream-proxy) behavior exactly.
func dialUpstream(dest string, timeout time.Duration) (net.Conn, error) {
	if cfg.UpstreamSocks5 == "" {
		return net.DialTimeout("tcp", dest, timeout)
	}
	return dialSocks5(cfg.UpstreamSocks5, cfg.UpstreamSocks5User, cfg.UpstreamSocks5Pass, dest, timeout)
}

// dialSocks5 performs a SOCKS5 CONNECT handshake against proxyAddr and
// returns a net.Conn that, once established, behaves like a plain TCP
// connection to dest (all subsequent reads/writes are the proxied traffic).
func dialSocks5(proxyAddr, user, pass, dest string, timeout time.Duration) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", proxyAddr, timeout)
	if err != nil {
		return nil, fmt.Errorf("dial upstream socks5 proxy %s: %w", proxyAddr, err)
	}

	if timeout > 0 {
		conn.SetDeadline(time.Now().Add(timeout))
		defer conn.SetDeadline(time.Time{}) // clear deadline once handshake is done
	}

	if err := socks5Handshake(conn, user, pass); err != nil {
		conn.Close()
		return nil, err
	}

	if err := socks5Connect(conn, dest); err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}

// socks5Handshake performs the initial method negotiation, offering
// username/password auth only if credentials were configured.
func socks5Handshake(conn net.Conn, user, pass string) error {
	methods := []byte{socks5AuthNone}
	if user != "" {
		methods = append(methods, socks5AuthUserPass)
	}

	greeting := make([]byte, 0, 2+len(methods))
	greeting = append(greeting, socks5Version, byte(len(methods)))
	greeting = append(greeting, methods...)
	if _, err := conn.Write(greeting); err != nil {
		return fmt.Errorf("socks5 greeting: %w", err)
	}

	resp := make([]byte, 2)
	if _, err := readFull(conn, resp); err != nil {
		return fmt.Errorf("socks5 greeting response: %w", err)
	}
	if resp[0] != socks5Version {
		return errors.New("socks5 greeting response: bad version")
	}

	switch resp[1] {
	case socks5AuthNone:
		return nil
	case socks5AuthUserPass:
		return socks5AuthenticateUserPass(conn, user, pass)
	case socks5AuthNoAccept:
		return errors.New("socks5 proxy rejected all auth methods (check credentials)")
	default:
		return fmt.Errorf("socks5 proxy selected unsupported auth method: 0x%02x", resp[1])
	}
}

// socks5AuthenticateUserPass implements RFC 1929 username/password auth.
func socks5AuthenticateUserPass(conn net.Conn, user, pass string) error {
	if len(user) > 255 || len(pass) > 255 {
		return errors.New("socks5 username/password too long (max 255 bytes each)")
	}

	req := new(bytes.Buffer)
	req.WriteByte(0x01) // auth subnegotiation version
	req.WriteByte(byte(len(user)))
	req.WriteString(user)
	req.WriteByte(byte(len(pass)))
	req.WriteString(pass)

	if _, err := conn.Write(req.Bytes()); err != nil {
		return fmt.Errorf("socks5 auth request: %w", err)
	}

	resp := make([]byte, 2)
	if _, err := readFull(conn, resp); err != nil {
		return fmt.Errorf("socks5 auth response: %w", err)
	}
	if resp[1] != socks5UserPassOK {
		return errors.New("socks5 proxy rejected username/password credentials")
	}
	return nil
}

// socks5Connect sends the CONNECT request for dest and waits for the reply.
func socks5Connect(conn net.Conn, dest string) error {
	host, portStr, err := net.SplitHostPort(dest)
	if err != nil {
		return fmt.Errorf("socks5 connect: invalid destination %q: %w", dest, err)
	}
	port, err := parsePort(portStr)
	if err != nil {
		return fmt.Errorf("socks5 connect: invalid port %q: %w", portStr, err)
	}

	req := new(bytes.Buffer)
	req.WriteByte(socks5Version)
	req.WriteByte(socks5CmdConnect)
	req.WriteByte(0x00) // reserved

	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			req.WriteByte(socks5AtypIPv4)
			req.Write(ip4)
		} else {
			req.WriteByte(socks5AtypIPv6)
			req.Write(ip.To16())
		}
	} else {
		if len(host) > 255 {
			return errors.New("socks5 connect: destination hostname too long")
		}
		req.WriteByte(socks5AtypDomain)
		req.WriteByte(byte(len(host)))
		req.WriteString(host)
	}
	req.WriteByte(byte(port >> 8))
	req.WriteByte(byte(port & 0xFF))

	if _, err := conn.Write(req.Bytes()); err != nil {
		return fmt.Errorf("socks5 connect request: %w", err)
	}

	// Reply header: VER, REP, RSV, ATYP
	head := make([]byte, 4)
	if _, err := readFull(conn, head); err != nil {
		return fmt.Errorf("socks5 connect response: %w", err)
	}
	if head[0] != socks5Version {
		return errors.New("socks5 connect response: bad version")
	}
	if head[1] != socks5ReplySucceded {
		return fmt.Errorf("socks5 connect failed: %s", socks5ReplyString(head[1]))
	}

	// Consume the bound address/port (length depends on ATYP) - we don't need it.
	switch head[3] {
	case socks5AtypIPv4:
		if _, err := readFull(conn, make([]byte, 4+2)); err != nil {
			return fmt.Errorf("socks5 connect response (ipv4 addr): %w", err)
		}
	case socks5AtypIPv6:
		if _, err := readFull(conn, make([]byte, 16+2)); err != nil {
			return fmt.Errorf("socks5 connect response (ipv6 addr): %w", err)
		}
	case socks5AtypDomain:
		lenBuf := make([]byte, 1)
		if _, err := readFull(conn, lenBuf); err != nil {
			return fmt.Errorf("socks5 connect response (domain len): %w", err)
		}
		if _, err := readFull(conn, make([]byte, int(lenBuf[0])+2)); err != nil {
			return fmt.Errorf("socks5 connect response (domain addr): %w", err)
		}
	default:
		return fmt.Errorf("socks5 connect response: unknown address type 0x%02x", head[3])
	}

	return nil
}

func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func parsePort(s string) (int, error) {
	var port int
	_, err := fmt.Sscanf(s, "%d", &port)
	if err != nil {
		return 0, err
	}
	if port <= 0 || port > 65535 {
		return 0, fmt.Errorf("port out of range: %d", port)
	}
	return port, nil
}

func socks5ReplyString(code byte) string {
	switch code {
	case 0x01:
		return "general SOCKS server failure"
	case 0x02:
		return "connection not allowed by ruleset"
	case 0x03:
		return "network unreachable"
	case 0x04:
		return "host unreachable"
	case 0x05:
		return "connection refused"
	case 0x06:
		return "TTL expired"
	case 0x07:
		return "command not supported"
	case 0x08:
		return "address type not supported"
	default:
		return fmt.Sprintf("unknown error 0x%02x", code)
	}
}
