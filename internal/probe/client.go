// Package probe owns network mechanics so providers only describe service signals.
package probe

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxBodyBytes = 2 << 20

// Family controls the address family used for outbound connections.
type Family string

const (
	Auto Family = "auto"
	IPv4 Family = "ipv4"
	IPv6 Family = "ipv6"
)

// Config configures a Client. Proxy may be http://, https://, socks5://, or socks5h://.
type Config struct {
	Family    Family
	Proxy     string
	Interface string
	SourceIP  string
	Timeout   time.Duration
	UserAgent string
}

// Response contains a bounded response body and the final URL after redirects.
type Response struct {
	StatusCode int
	URL        string
	Header     http.Header
	Body       []byte
}

// Client is safe for concurrent use.
type Client struct {
	http      *http.Client
	userAgent string
}

// New validates configuration and returns an HTTP client with bounded redirects.
func New(cfg Config) (*Client, error) {
	family := cfg.Family
	if family == "" {
		family = Auto
	}
	if family != Auto && family != IPv4 && family != IPv6 {
		return nil, fmt.Errorf("invalid IP family %q (want auto, ipv4, or ipv6)", family)
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 12 * time.Second
	}
	localIP, err := resolveSourceIP(cfg.Interface, cfg.SourceIP, family)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          32,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   cfg.Timeout,
		ResponseHeaderTimeout: cfg.Timeout,
		ExpectContinueTimeout: 1 * time.Second,
	}
	dialer := &net.Dialer{Timeout: cfg.Timeout, KeepAlive: 30 * time.Second}
	if localIP != nil {
		dialer.LocalAddr = &net.TCPAddr{IP: localIP}
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if family == IPv4 {
			network = "tcp4"
		} else if family == IPv6 {
			network = "tcp6"
		} else if strings.HasPrefix(network, "tcp") {
			network = "tcp"
		}
		return dialer.DialContext(ctx, network, address)
	}
	if cfg.Proxy != "" {
		u, err := url.Parse(cfg.Proxy)
		if err != nil || u.Scheme == "" {
			return nil, fmt.Errorf("invalid proxy URL %q", cfg.Proxy)
		}
		switch strings.ToLower(u.Scheme) {
		case "http", "https":
			transport.Proxy = http.ProxyURL(u)
		case "socks5", "socks5h":
			proxyAddr := u.Host
			if proxyAddr == "" {
				return nil, fmt.Errorf("proxy URL %q has no host", cfg.Proxy)
			}
			transport.Proxy = nil
			transport.DialContext = socks5Dialer(dialer, proxyAddr, u.User, family)
		default:
			return nil, fmt.Errorf("unsupported proxy scheme %q", u.Scheme)
		}
	}
	ua := cfg.UserAgent
	if ua == "" {
		ua = "unlockscope/0.1 (+https://github.com/shui1iao/UnlockScope)"
	}
	return &Client{http: &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("redirect limit exceeded")
			}
			req.Header.Set("User-Agent", ua)
			return nil
		},
	}, userAgent: ua}, nil
}

func resolveSourceIP(interfaceName, source string, family Family) (net.IP, error) {
	if source != "" {
		ip := net.ParseIP(source)
		if ip == nil {
			return nil, fmt.Errorf("invalid source IP %q", source)
		}
		if !familyMatches(ip, family) {
			return nil, fmt.Errorf("source IP %q does not match family %q", source, family)
		}
		return ip, nil
	}
	if interfaceName == "" {
		return nil, nil
	}
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return nil, fmt.Errorf("find interface %q: %w", interfaceName, err)
	}
	addresses, err := iface.Addrs()
	if err != nil {
		return nil, fmt.Errorf("list interface %q addresses: %w", interfaceName, err)
	}
	for _, address := range addresses {
		ip, _, parseErr := net.ParseCIDR(address.String())
		if parseErr == nil && !ip.IsLoopback() && familyMatches(ip, family) {
			return ip, nil
		}
	}
	return nil, fmt.Errorf("interface %q has no address matching family %q", interfaceName, family)
}

func familyMatches(ip net.IP, family Family) bool {
	switch family {
	case IPv4:
		return ip.To4() != nil
	case IPv6:
		return ip.To4() == nil
	default:
		return true
	}
}

// Get performs a GET and caps the body to avoid provider pages consuming memory.
func (c *Client) Get(ctx context.Context, target string) (Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html,application/json;q=0.9,*/*;q=0.5")
	resp, err := c.http.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return Response{StatusCode: resp.StatusCode, URL: resp.Request.URL.String(), Header: resp.Header.Clone()}, err
	}
	if len(body) > maxBodyBytes {
		body = body[:maxBodyBytes]
	}
	return Response{StatusCode: resp.StatusCode, URL: resp.Request.URL.String(), Header: resp.Header.Clone(), Body: body}, nil
}

// JSON decodes a response body into a generic JSON value.
func (c *Client) JSON(ctx context.Context, target string) (Response, any, error) {
	resp, err := c.Get(ctx, target)
	if err != nil {
		return resp, nil, err
	}
	var value any
	if err := json.Unmarshal(resp.Body, &value); err != nil {
		return resp, nil, err
	}
	return resp, value, nil
}

// DetectRegion uses a public, no-credential geolocation endpoint. An empty
// result is deliberately non-fatal: automatic selection then uses global checks.
func (c *Client) DetectRegion(ctx context.Context) (string, error) {
	resp, value, err := c.JSON(ctx, "https://ipapi.co/json/")
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("geo endpoint returned HTTP %d", resp.StatusCode)
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return "", errors.New("geo endpoint returned non-object JSON")
	}
	country, _ := obj["country_code"].(string)
	country = strings.ToLower(strings.TrimSpace(country))
	if len(country) != 2 {
		return "", errors.New("geo endpoint omitted country_code")
	}
	return country, nil
}

func socks5Dialer(base *net.Dialer, proxyAddr string, auth *url.Userinfo, family Family) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		if family == IPv4 {
			network = "tcp4"
		} else if family == IPv6 {
			network = "tcp6"
		} else {
			network = "tcp"
		}
		conn, err := base.DialContext(ctx, network, proxyAddr)
		if err != nil {
			return nil, err
		}
		closeOnErr := func(e error) (net.Conn, error) { _ = conn.Close(); return nil, e }
		deadline, hasDeadline := ctx.Deadline()
		if hasDeadline {
			_ = conn.SetDeadline(deadline)
		}
		methods := []byte{0}
		if auth != nil {
			methods = []byte{0, 2}
		}
		if _, err = conn.Write(append([]byte{5, byte(len(methods))}, methods...)); err != nil {
			return closeOnErr(err)
		}
		var greeting [2]byte
		if _, err = io.ReadFull(conn, greeting[:]); err != nil {
			return closeOnErr(err)
		}
		if greeting[0] != 5 || greeting[1] == 255 {
			return closeOnErr(errors.New("socks5 proxy rejected authentication methods"))
		}
		if greeting[1] == 2 {
			user, pass := auth.Username(), ""
			if p, ok := auth.Password(); ok {
				pass = p
			}
			if len(user) > 255 || len(pass) > 255 {
				return closeOnErr(errors.New("socks5 credentials too long"))
			}
			msg := append([]byte{1, byte(len(user))}, []byte(user)...)
			msg = append(msg, byte(len(pass)))
			msg = append(msg, []byte(pass)...)
			if _, err = conn.Write(msg); err != nil {
				return closeOnErr(err)
			}
			var authResp [2]byte
			if _, err = io.ReadFull(conn, authResp[:]); err != nil {
				return closeOnErr(err)
			}
			if authResp[1] != 0 {
				return closeOnErr(errors.New("socks5 proxy authentication failed"))
			}
		}
		host, portText, err := net.SplitHostPort(address)
		if err != nil {
			return closeOnErr(err)
		}
		port, err := net.LookupPort("tcp", portText)
		if err != nil {
			return closeOnErr(err)
		}
		request := []byte{5, 1, 0}
		ip := net.ParseIP(host)
		if ip4 := ip.To4(); ip4 != nil {
			request = append(request, 1)
			request = append(request, ip4...)
		} else if ip != nil {
			request = append(request, 4)
			request = append(request, ip.To16()...)
		} else {
			hostBytes := []byte(host)
			if len(hostBytes) > 255 {
				return closeOnErr(errors.New("socks5 destination hostname too long"))
			}
			request = append(request, 3, byte(len(hostBytes)))
			request = append(request, hostBytes...)
		}
		var portBuf [2]byte
		binary.BigEndian.PutUint16(portBuf[:], uint16(port))
		request = append(request, portBuf[:]...)
		if _, err = conn.Write(request); err != nil {
			return closeOnErr(err)
		}
		var head [4]byte
		if _, err = io.ReadFull(conn, head[:]); err != nil {
			return closeOnErr(err)
		}
		if head[1] != 0 {
			return closeOnErr(fmt.Errorf("socks5 proxy connect failed with code %d", head[1]))
		}
		switch head[3] {
		case 1:
			var tail [6]byte
			if _, err = io.ReadFull(conn, tail[:]); err != nil {
				return closeOnErr(err)
			}
		case 4:
			var tail [18]byte
			if _, err = io.ReadFull(conn, tail[:]); err != nil {
				return closeOnErr(err)
			}
		case 3:
			var length [1]byte
			if _, err = io.ReadFull(conn, length[:]); err != nil {
				return closeOnErr(err)
			}
			tail := make([]byte, int(length[0])+2)
			if _, err = io.ReadFull(conn, tail); err != nil {
				return closeOnErr(err)
			}
		default:
			return closeOnErr(errors.New("socks5 proxy returned invalid address type"))
		}
		_ = conn.SetDeadline(time.Time{})
		return conn, nil
	}
}
