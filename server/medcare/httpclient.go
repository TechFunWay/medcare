package medcare

import (
    "context"
    "net"
    "net/http"
    "os"
    "strings"
    "sync"
    "time"
)

// 有些机器（如本机常驻的 fake-IP 代理）会把系统 DNS 指到 198.18.x 的假地址，
// 而 Go 不读系统代理，直连假 IP 必然 TLS 握手失败——表现为「AI 服务连接失败：
// TLS handshake timeout」，但同一域名用真实 DNS 解析后直连是通的（实测 200）。
// 这里支持用 MEDCARE_DNS_SERVERS 指定 DNS（逗号分隔，如 223.5.5.5 或 1.1.1.1:53），
// 设置后 AI / OCR 的 HTTP 客户端会用它解析域名再直连；不设置则完全保持原行为。
var (
    appTransportOnce sync.Once
    appTransportInst *http.Transport
)

func dnsServers() []string {
    raw := strings.TrimSpace(os.Getenv("MEDCARE_DNS_SERVERS"))
    if raw == "" {
        return nil
    }
    var out []string
    for _, s := range strings.Split(raw, ",") {
        s = strings.TrimSpace(s)
        if s == "" {
            continue
        }
        if !strings.Contains(s, ":") {
            s += ":53"
        }
        out = append(out, s)
    }
    return out
}

// appResolver 返回按 MEDCARE_DNS_SERVERS 配置的解析器；未配置返回 nil。
func appResolver() *net.Resolver {
    servers := dnsServers()
    if len(servers) == 0 {
        return nil
    }
    idx := 0
    return &net.Resolver{
        PreferGo: true,
        Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
            // 多个 DNS 轮询，单个失败换下一个
            srv := servers[idx%len(servers)]
            idx++
            d := net.Dialer{Timeout: 5 * time.Second}
            return d.DialContext(ctx, "udp", srv)
        },
    }
}

// appHTTPTransport 返回给外部 API 用的 Transport：配置了自定义 DNS 时，
// 先解析出真实 IP 再直连（TLS 仍用原域名做 SNI/证书校验）。
func appHTTPTransport() *http.Transport {
    appTransportOnce.Do(func() {
        tr := &http.Transport{Proxy: http.ProxyFromEnvironment}
        r := appResolver()
        if r != nil {
            tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
                host, port, err := net.SplitHostPort(addr)
                if err != nil {
                    return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, addr)
                }
                // IP 直接拨号，域名先按自定义 DNS 解析
                if net.ParseIP(host) != nil {
                    return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, addr)
                }
                lookupCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
                defer cancel()
                ips, err := r.LookupHost(lookupCtx, host)
                if err != nil || len(ips) == 0 {
                    // 解析失败退回系统解析，避免整体不可用
                    return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, addr)
                }
                var lastErr error
                for _, ip := range ips {
                    conn, derr := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip, port))
                    if derr == nil {
                        return conn, nil
                    }
                    lastErr = derr
                }
                return nil, lastErr
            }
        }
        appTransportInst = tr
    })
    return appTransportInst
}
