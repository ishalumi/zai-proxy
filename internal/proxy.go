package internal

import (
	"net/http"
	"net/url"
)

// GetProxyClient 返回配置了代理的 HTTP Client
func GetProxyClient() *http.Client {
	if Cfg.ProxyURL == "" {
		return &http.Client{}
	}

	proxyURL, err := url.Parse(Cfg.ProxyURL)
	if err != nil {
		LogError("Invalid proxy URL: %v", err)
		return &http.Client{}
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
	}

	return &http.Client{
		Transport: transport,
	}
}
