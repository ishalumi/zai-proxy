package internal

import (
	"hash/fnv"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"time"
)

var (
	defaultClient     *http.Client
	proxyClient       *http.Client
	defaultClientOnce sync.Once
	proxyClientOnce   sync.Once
	randOnce          sync.Once
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

// GetProxyClient 返回强制走代理的 HTTP Client (单例)
func GetProxyClient() *http.Client {
	if Cfg.ProxyURL == "" {
		return GetDefaultClient()
	}
	proxyClientOnce.Do(func() {
		proxyClient = newClient(Cfg.ProxyURL)
	})
	return proxyClient
}

// GetDefaultClient 返回直连的 HTTP Client (单例)
func GetDefaultClient() *http.Client {
	defaultClientOnce.Do(func() {
		defaultClient = newClient("")
	})
	return defaultClient
}

// GetStickyProxyClient 根据 key (如 token) 决定使用直连还是代理
// 保证同一个 key 在配置不变的情况下始终走同一条路径，避免 IP 跳变触发风控
func GetStickyProxyClient(key string) *http.Client {
	if Cfg.ProxyURL == "" {
		return GetDefaultClient()
	}

	if key == "" {
		return GetRandomProxyClient()
	}

	h := fnv.New32a()
	h.Write([]byte(key))
	if h.Sum32()%2 == 0 {
		LogDebug("Sticky Proxy: Using direct connection for key: %s", key[:min(10, len(key))])
		return GetDefaultClient()
	}

	LogDebug("Sticky Proxy: Using proxy connection for key: %s", key[:min(10, len(key))])
	return GetProxyClient()
}

// GetRandomProxyClient 随机决定是直连还是走代理 (50% 概率)
func GetRandomProxyClient() *http.Client {
	if Cfg.ProxyURL == "" {
		return GetDefaultClient()
	}

	randOnce.Do(func() {
		rand.Seed(time.Now().UnixNano())
	})

	if rand.Float64() < 0.5 {
		LogDebug("Random Proxy: Using direct connection")
		return GetDefaultClient()
	}

	LogDebug("Random Proxy: Using proxy connection")
	return GetProxyClient()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
