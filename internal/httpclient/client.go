package httpclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

type safeTransport struct {
	base   *http.Transport
	lookup func(context.Context, string) ([]net.IPAddr, error)
	dial   func(context.Context, string, string) (net.Conn, error)
}

func New(timeout time.Duration) *http.Client {
	base, ok := http.DefaultTransport.(*http.Transport)
	if ok {
		base = base.Clone()
	} else {
		base = &http.Transport{}
	}
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	transport := &safeTransport{base: base, lookup: net.DefaultResolver.LookupIPAddr, dial: dialer.DialContext}
	base.DialContext = transport.dialContext
	base.DialTLSContext = nil
	base.DialTLS = nil
	return &http.Client{
		Transport:     transport,
		Timeout:       timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func (transport *safeTransport) addresses(ctx context.Context, host string) ([]net.IPAddr, error) {
	addresses, err := transport.lookup(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, errors.New("destination has no addresses")
	}
	for _, address := range addresses {
		if address.IP.IsLinkLocalUnicast() || address.IP.IsUnspecified() || address.IP.IsMulticast() {
			return nil, errors.New("destination resolves to a prohibited address")
		}
	}
	return addresses, nil
}

func (transport *safeTransport) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := transport.addresses(ctx, host)
	if err != nil {
		return nil, err
	}
	var failures []error
	for _, resolved := range addresses {
		connection, err := transport.dial(ctx, network, net.JoinHostPort(resolved.String(), port))
		if err == nil {
			return connection, nil
		}
		failures = append(failures, err)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, errors.Join(failures...)
}

func (transport *safeTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL == nil || request.URL.Hostname() == "" {
		return nil, fmt.Errorf("request host is missing")
	}
	if _, err := transport.addresses(request.Context(), request.URL.Hostname()); err != nil {
		return nil, err
	}
	return transport.base.RoundTrip(request)
}

func (transport *safeTransport) CloseIdleConnections() {
	transport.base.CloseIdleConnections()
}
