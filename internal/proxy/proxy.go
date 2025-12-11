package proxy

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"ruoyi-proxy/internal/config"
)

// ServiceProxy 单个服务的代理
type ServiceProxy struct {
	BlueProxy  *httputil.ReverseProxy
	GreenProxy *httputil.ReverseProxy
}

// Proxy 多服务代理结构
type Proxy struct {
	mu       sync.RWMutex
	config   *config.Config
	services map[string]*ServiceProxy // key: serviceID
}

// New 初始化代理
func New() (*Proxy, error) {
	p := &Proxy{
		services: make(map[string]*ServiceProxy),
	}

	// 加载初始配置
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %v", err)
	}
	p.config = cfg

	// 为每个服务创建反向代理
	for serviceID, svcCfg := range cfg.Services {
		sp := &ServiceProxy{}

		sp.BlueProxy, err = createProxy(svcCfg.BlueTarget)
		if err != nil {
			return nil, fmt.Errorf("创建服务[%s]蓝色代理失败: %v", serviceID, err)
		}

		sp.GreenProxy, err = createProxy(svcCfg.GreenTarget)
		if err != nil {
			return nil, fmt.Errorf("创建服务[%s]绿色代理失败: %v", serviceID, err)
		}

		p.services[serviceID] = sp
		log.Printf("服务[%s](%s) 初始化完成 - 蓝: %s, 绿: %s, 活跃: %s",
			serviceID, svcCfg.Name, svcCfg.BlueTarget, svcCfg.GreenTarget, svcCfg.ActiveEnv)
	}

	log.Printf("代理初始化完成，共 %d 个服务", len(p.services))
	return p, nil
}

// createProxy 创建反向代理
func createProxy(target string) (*httputil.ReverseProxy, error) {
	targetURL, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("解析目标URL失败: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	proxy.Transport = &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		DisableKeepAlives:     false,
		DisableCompression:    true,
		ResponseHeaderTimeout: 900 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("代理错误: %v, URL: %s", err, r.URL.String())
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, "代理服务暂不可用: %v", err)
	}

	return proxy, nil
}

// HandleProxy 代理请求处理（根据URL路径识别服务）
// 路由规则: /api/{serviceID}/... -> 对应服务
func (p *Proxy) HandleProxy(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// 解析服务ID，格式: /api/{serviceID}/...
	serviceID := p.extractServiceID(r.URL.Path)

	svcCfg := p.config.GetService(serviceID)
	if svcCfg == nil {
		// 如果未匹配到特定服务，回退到默认服务
		serviceID = "default"
		svcCfg = p.config.Services["default"]
	}

	if svcCfg == nil {
		// 如果连默认服务都没有（配置严重错误），尝试降级到任意一个可用服务
		for id := range p.config.Services {
			serviceID = id
			svcCfg = p.config.Services[id]
			break
		}
	}

	if svcCfg == nil {
		http.Error(w, "未配置服务", http.StatusNotFound)
		return
	}

	sp := p.services[serviceID]
	if sp == nil {
		http.Error(w, "服务未初始化", http.StatusInternalServerError)
		return
	}

	var proxy *httputil.ReverseProxy
	switch svcCfg.ActiveEnv {
	case "green":
		proxy = sp.GreenProxy
	default:
		proxy = sp.BlueProxy
	}

	// 如果是通过URL路径匹配到的特定服务（非default回退），需要去除服务ID前缀
	// 例如: /api/collect/list -> /api/list (假设collect是服务ID)
	if svcCfg != nil && serviceID != "default" {
		// 简单的路径重写逻辑
		// 1. 处理 /api/{serviceID} 的情况
		prefixApi := "/api/" + serviceID
		if strings.HasPrefix(r.URL.Path, prefixApi) {
			newPath := "/api" + strings.TrimPrefix(r.URL.Path, prefixApi)
			r.URL.Path = newPath
			if r.URL.RawPath != "" {
				r.URL.RawPath = "/api" + strings.TrimPrefix(r.URL.RawPath, prefixApi)
			}
		} else {
			// 2. 处理 /{serviceID} 的情况
			prefixRoot := "/" + serviceID
			if strings.HasPrefix(r.URL.Path, prefixRoot) {
				newPath := strings.TrimPrefix(r.URL.Path, prefixRoot)
				if !strings.HasPrefix(newPath, "/") {
					newPath = "/" + newPath
				}
				r.URL.Path = newPath
				if r.URL.RawPath != "" {
					// 简单处理RawPath，实际场景可能需要更复杂的转义处理
					r.URL.RawPath = newPath
				}
			}
		}
		log.Printf("[Proxy] Rewriting Path for service %s: %s -> %s", serviceID, r.RequestURI, r.URL.Path)
	}

	// 添加调试头
	r.Header.Set("X-Proxy-Service", serviceID)
	r.Header.Set("X-Proxy-Env", svcCfg.ActiveEnv)
	r.Header.Set("X-Proxy-Time", time.Now().Format("2006-01-02 15:04:05"))

	proxy.ServeHTTP(w, r)
}

// extractServiceID 从路径提取服务ID
// 支持格式: /api/{serviceID}/... 或 /{serviceID}/...
func (p *Proxy) extractServiceID(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 2 && parts[0] == "api" {
		return parts[1]
	}
	if len(parts) >= 1 {
		// 检查第一段是否是已知服务
		if _, ok := p.config.Services[parts[0]]; ok {
			return parts[0]
		}
	}
	return ""
}

// SwitchService 切换指定服务的环境
func (p *Proxy) SwitchService(serviceID, env string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	svc := p.config.GetService(serviceID)
	if svc == nil {
		return fmt.Errorf("服务不存在: %s", serviceID)
	}

	svc.ActiveEnv = env
	return config.SaveConfig(p.config)
}

// SwitchAll 切换所有服务的环境
func (p *Proxy) SwitchAll(env string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, svc := range p.config.Services {
		svc.ActiveEnv = env
	}
	return config.SaveConfig(p.config)
}

// AddService 添加新服务
func (p *Proxy) AddService(serviceID string, svcCfg *config.ServiceConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 检查服务是否已存在
	if _, exists := p.config.Services[serviceID]; exists {
		return fmt.Errorf("服务[%s]已存在", serviceID)
	}

	// 创建代理
	sp := &ServiceProxy{}
	var err error

	sp.BlueProxy, err = createProxy(svcCfg.BlueTarget)
	if err != nil {
		return fmt.Errorf("创建蓝色代理失败: %v", err)
	}

	sp.GreenProxy, err = createProxy(svcCfg.GreenTarget)
	if err != nil {
		return fmt.Errorf("创建绿色代理失败: %v", err)
	}

	p.config.Services[serviceID] = svcCfg
	p.services[serviceID] = sp

	log.Printf("服务[%s](%s) 已添加 - 蓝: %s, 绿: %s",
		serviceID, svcCfg.Name, svcCfg.BlueTarget, svcCfg.GreenTarget)

	return config.SaveConfig(p.config)
}

// RemoveService 删除服务
func (p *Proxy) RemoveService(serviceID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.config.Services[serviceID]; !exists {
		return fmt.Errorf("服务[%s]不存在", serviceID)
	}

	// 确保至少保留一个服务
	if len(p.config.Services) <= 1 {
		return fmt.Errorf("至少需要保留一个服务")
	}

	delete(p.config.Services, serviceID)
	delete(p.services, serviceID)

	log.Printf("服务[%s] 已删除", serviceID)

	return config.SaveConfig(p.config)
}

// GetConfig 获取当前配置
func (p *Proxy) GetConfig() *config.Config {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.config
}

// UpdateConfig 更新配置并重建代理
func (p *Proxy) UpdateConfig(cfg *config.Config) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 为新配置中的服务创建代理
	newServices := make(map[string]*ServiceProxy)
	for serviceID, svcCfg := range cfg.Services {
		sp := &ServiceProxy{}
		var err error

		sp.BlueProxy, err = createProxy(svcCfg.BlueTarget)
		if err != nil {
			return fmt.Errorf("创建服务[%s]蓝色代理失败: %v", serviceID, err)
		}

		sp.GreenProxy, err = createProxy(svcCfg.GreenTarget)
		if err != nil {
			return fmt.Errorf("创建服务[%s]绿色代理失败: %v", serviceID, err)
		}

		newServices[serviceID] = sp
		log.Printf("服务[%s](%s) 代理已重建", serviceID, svcCfg.Name)
	}

	p.config = cfg
	p.services = newServices

	return config.SaveConfig(cfg)
}
