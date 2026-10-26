package websearch

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	golangproxy "golang.org/x/net/proxy"
)

const (
	requestTimeout  = 10 * time.Second
	maxResponseSize = 4 << 20
)

func newHTTPClient(proxyText string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	if proxyText == "" {
		return &http.Client{Transport: transport, Timeout: requestTimeout}, nil
	}
	proxyURL, err := validateProxy(proxyText)
	if err != nil {
		return nil, err
	}
	switch proxyURL.Scheme {
	case "http":
		transport.Proxy = http.ProxyURL(proxyURL)
	case "socks5", "socks5h":
		dialer, dialErr := golangproxy.FromURL(proxyURL, &net.Dialer{Timeout: requestTimeout, KeepAlive: 30 * time.Second})
		if dialErr != nil {
			return nil, wrap("invalid_input", "invalid proxy URL", dialErr)
		}
		if contextDialer, ok := dialer.(golangproxy.ContextDialer); ok {
			transport.DialContext = contextDialer.DialContext
		} else {
			transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
				return dialWithContext(ctx, dialer, network, address)
			}
		}
	}
	return &http.Client{Transport: transport, Timeout: requestTimeout}, nil
}

func validateProxy(value string) (*url.URL, error) {
	proxyURL, err := url.Parse(value)
	if err != nil || proxyURL.Hostname() == "" || proxyURL.Port() == "" {
		return nil, errorf("invalid_input", "proxy must use http://host:port, socks5://host:port, or socks5h://host:port format")
	}
	if proxyURL.Scheme != "http" && proxyURL.Scheme != "socks5" && proxyURL.Scheme != "socks5h" {
		return nil, errorf("invalid_input", "proxy must use http://host:port, socks5://host:port, or socks5h://host:port format")
	}
	if proxyURL.Path != "" && proxyURL.Path != "/" || proxyURL.RawQuery != "" || proxyURL.Fragment != "" {
		return nil, errorf("invalid_input", "proxy must use http://host:port, socks5://host:port, or socks5h://host:port format")
	}
	port, err := strconv.Atoi(proxyURL.Port())
	if err != nil || port < 1 || port > 65535 {
		return nil, errorf("invalid_input", "proxy must use http://host:port, socks5://host:port, or socks5h://host:port format")
	}
	return proxyURL, nil
}

func dialWithContext(ctx context.Context, dialer golangproxy.Dialer, network, address string) (net.Conn, error) {
	type result struct {
		conn net.Conn
		err  error
	}
	done := make(chan result)
	go func() {
		conn, err := dialer.Dial(network, address)
		select {
		case done <- result{conn: conn, err: err}:
		case <-ctx.Done():
			if conn != nil {
				_ = conn.Close()
			}
		}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-done:
		return result.conn, result.err
	}
}

func fetchHTML(ctx context.Context, client *http.Client, endpoint string, params url.Values, headers http.Header) ([]byte, error) {
	requestURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	requestURL.RawQuery = params.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header = headers.Clone()
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search backend returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxResponseSize {
		return nil, fmt.Errorf("search backend response exceeds %d bytes", maxResponseSize)
	}
	return body, nil
}

func pageCount(maxResults int) int {
	return 1 + (maxResults-1)/10
}

func appendPage(results []Result, page []Result, seen map[string]struct{}, maximum int) ([]Result, int) {
	added := 0
	for _, result := range page {
		key := strings.TrimSpace(result.Href)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		results = append(results, result)
		added++
		if len(results) >= maximum {
			break
		}
	}
	return results, added
}
