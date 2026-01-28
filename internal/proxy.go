package internal

import (
	"net/http"
	"net/url"
	"sync"
)

var (
	proxyClient     *http.Client
	proxyClientOnce sync.Once
)

func newClient(proxyStr string) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 100

	if proxyStr != "" {
		proxyURL, err := url.Parse(proxyStr)
		if err != nil {
			LogError("Invalid proxy URL: %v", err)
		} else {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	return &http.Client{
		Transport: transport,
	}
}

// GetProxyClient 返回走代理的 HTTP Client (单例)
func GetProxyClient() *http.Client {
	proxyClientOnce.Do(func() {
		proxyClient = newClient(Cfg.ProxyURL)
	})
	return proxyClient
}

// GetStickyProxyClient 保持接口兼容，统一走代理
func GetStickyProxyClient(key string) *http.Client {
	return GetProxyClient()
}

// GetRandomProxyClient 保持接口兼容，统一走代理
func GetRandomProxyClient() *http.Client {
	return GetProxyClient()
}

// GetDefaultClient 保持接口兼容，统一走代理
func GetDefaultClient() *http.Client {
	return GetProxyClient()
}
