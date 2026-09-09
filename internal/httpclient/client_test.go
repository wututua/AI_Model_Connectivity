package httpclient

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
)

func TestDialRejectsUnsafeAddressesBeforeConnecting(t *testing.T) {
	for _, address := range []string{"169.254.169.254", "fe80::1", "::ffff:169.254.169.254", "0.0.0.0", "ff02::1"} {
		transport := &safeTransport{
			lookup: func(context.Context, string) ([]net.IPAddr, error) {
				return []net.IPAddr{{IP: net.ParseIP(address)}}, nil
			},
			dial: func(context.Context, string, string) (net.Conn, error) {
				t.Fatal("unsafe address was dialed")
				return nil, nil
			},
		}
		if _, err := transport.dialContext(context.Background(), "tcp", "example.test:80"); err == nil {
			t.Errorf("accepted %s", address)
		}
	}
}

func TestDialPinsResolvedAddress(t *testing.T) {
	lookups := 0
	transport := &safeTransport{
		lookup: func(context.Context, string) ([]net.IPAddr, error) {
			lookups++
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		},
		dial: func(_ context.Context, _ string, address string) (net.Conn, error) {
			if address != "127.0.0.1:8080" {
				t.Errorf("dial did not pin IP: %s", address)
			}
			return nil, errors.New("test connection")
		},
	}
	transport.dialContext(context.Background(), "tcp", "example.test:8080")
	if lookups != 1 {
		t.Fatalf("unexpected DNS lookups: %d", lookups)
	}
}

func TestDialRechecksAfterDNSChanges(t *testing.T) {
	lookups := 0
	transport := &safeTransport{
		lookup: func(context.Context, string) ([]net.IPAddr, error) {
			lookups++
			address := "127.0.0.1"
			if lookups > 1 {
				address = "169.254.169.254"
			}
			return []net.IPAddr{{IP: net.ParseIP(address)}}, nil
		},
		dial: func(context.Context, string, string) (net.Conn, error) {
			t.Fatal("rebound address was dialed")
			return nil, nil
		},
	}
	transport.base = &http.Transport{DialContext: transport.dialContext}
	defer transport.CloseIdleConnections()
	request, _ := http.NewRequest(http.MethodGet, "http://example.test", nil)
	if _, err := transport.RoundTrip(request); err == nil {
		t.Fatal("DNS rebinding was accepted")
	}
}
