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
	mu         sync.RWMutex
	config     *config.Config
	services   map[string]*ServiceProxy // key: serviceID
	saveConfig func(*config.Config) error
}

// New 初始化代理
func New() (*Proxy, error) {
	p := &Proxy{
		services:   make(map[string]*ServiceProxy),
		saveConfig: config.SaveConfig,
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

// routeSnapshot 单次请求使用的不可变路由快照。
type routeSnapshot struct {
	serviceID string
	activeEnv string
	proxy     *httputil.ReverseProxy
}

// HandleProxy 代理请求处理（根据URL路径识别服务）
// 路由规则: /api/{serviceID}/... -> 对应服务
func (p *Proxy) HandleProxy(w http.ResponseWriter, r *http.Request) {
	route, status, err := p.resolveRoute(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}

	// 如果是通过URL路径匹配到的特定服务（非default回退），需要去除服务ID前缀。
	// 例如: /api/collect/list -> /api/list (假设collect是服务ID)
	if route.serviceID != "default" {
		prefixAPI := "/api/" + route.serviceID
		if strings.HasPrefix(r.URL.Path, prefixAPI) {
			newPath := "/api" + strings.TrimPrefix(r.URL.Path, prefixAPI)
			r.URL.Path = newPath
			if r.URL.RawPath != "" {
				r.URL.RawPath = "/api" + strings.TrimPrefix(r.URL.RawPath, prefixAPI)
			}
		} else {
			prefixRoot := "/" + route.serviceID
			if strings.HasPrefix(r.URL.Path, prefixRoot) {
				newPath := strings.TrimPrefix(r.URL.Path, prefixRoot)
				if !strings.HasPrefix(newPath, "/") {
					newPath = "/" + newPath
				}
				r.URL.Path = newPath
				if r.URL.RawPath != "" {
					r.URL.RawPath = newPath
				}
			}
		}
		log.Printf("[Proxy] Rewriting Path for service %s: %s -> %s", route.serviceID, r.RequestURI, r.URL.Path)
	}

	r.Header.Set("X-Proxy-Service", route.serviceID)
	r.Header.Set("X-Proxy-Env", route.activeEnv)
	r.Header.Set("X-Proxy-Time", time.Now().Format("2006-01-02 15:04:05"))

	// 上游请求可能是 SSE/WebSocket 或其他长连接，不能在此期间持有配置读锁。
	route.proxy.ServeHTTP(w, r)
}

// resolveRoute 在读锁内解析并复制当前请求需要的路由状态。
func (p *Proxy) resolveRoute(path string) (routeSnapshot, int, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	serviceID := p.extractServiceIDLocked(path)
	svcCfg := p.config.GetService(serviceID)
	if svcCfg == nil {
		serviceID = "default"
		svcCfg = p.config.Services["default"]
	}
	if svcCfg == nil {
		for id, candidate := range p.config.Services {
			serviceID = id
			svcCfg = candidate
			break
		}
	}
	if svcCfg == nil {
		return routeSnapshot{}, http.StatusNotFound, fmt.Errorf("未配置服务")
	}

	serviceProxy := p.services[serviceID]
	if serviceProxy == nil {
		return routeSnapshot{}, http.StatusInternalServerError, fmt.Errorf("服务未初始化")
	}

	target := serviceProxy.BlueProxy
	if svcCfg.ActiveEnv == "green" {
		target = serviceProxy.GreenProxy
	}
	if target == nil {
		return routeSnapshot{}, http.StatusInternalServerError, fmt.Errorf("目标代理未初始化")
	}
	return routeSnapshot{serviceID: serviceID, activeEnv: svcCfg.ActiveEnv, proxy: target}, http.StatusOK, nil
}

// extractServiceIDLocked 从路径提取服务ID，调用方必须持有读锁或写锁。
// 支持格式: /api/{serviceID}/... 或 /{serviceID}/...
func (p *Proxy) extractServiceIDLocked(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 2 && parts[0] == "api" {
		return parts[1]
	}
	if len(parts) >= 1 {
		if _, ok := p.config.Services[parts[0]]; ok {
			return parts[0]
		}
	}
	return ""
}

// SwitchService 切换指定服务的环境
func (p *Proxy) SwitchService(serviceID, env string) error {
	if !isValidEnvironment(env) {
		return fmt.Errorf("无效环境: %s", env)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	candidate := p.config.Clone()
	svc := candidate.GetService(serviceID)
	if svc == nil {
		return fmt.Errorf("服务不存在: %s", serviceID)
	}
	svc.ActiveEnv = env
	if err := p.saveConfigLocked(candidate); err != nil {
		return err
	}
	p.config = candidate
	return nil
}

// SwitchAll 切换所有服务的环境
func (p *Proxy) SwitchAll(env string) error {
	if !isValidEnvironment(env) {
		return fmt.Errorf("无效环境: %s", env)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	candidate := p.config.Clone()
	for _, svc := range candidate.Services {
		if svc != nil {
			svc.ActiveEnv = env
		}
	}
	if err := p.saveConfigLocked(candidate); err != nil {
		return err
	}
	p.config = candidate
	return nil
}

// AddService 添加新服务
func (p *Proxy) AddService(serviceID string, svcCfg *config.ServiceConfig) error {
	if svcCfg == nil {
		return fmt.Errorf("服务配置不能为空")
	}

	serviceProxy := &ServiceProxy{}
	var err error
	serviceProxy.BlueProxy, err = createProxy(svcCfg.BlueTarget)
	if err != nil {
		return fmt.Errorf("创建蓝色代理失败: %v", err)
	}
	serviceProxy.GreenProxy, err = createProxy(svcCfg.GreenTarget)
	if err != nil {
		return fmt.Errorf("创建绿色代理失败: %v", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.config.Services[serviceID]; exists {
		return fmt.Errorf("服务[%s]已存在", serviceID)
	}

	candidate := p.config.Clone()
	serviceCopy := *svcCfg
	candidate.Services[serviceID] = &serviceCopy
	if err := p.saveConfigLocked(candidate); err != nil {
		return err
	}

	newServices := cloneServiceProxies(p.services)
	newServices[serviceID] = serviceProxy
	p.config = candidate
	p.services = newServices
	log.Printf("服务[%s](%s) 已添加 - 蓝: %s, 绿: %s", serviceID, svcCfg.Name, svcCfg.BlueTarget, svcCfg.GreenTarget)
	return nil
}

// RemoveService 删除服务
func (p *Proxy) RemoveService(serviceID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.config.Services[serviceID]; !exists {
		return fmt.Errorf("服务[%s]不存在", serviceID)
	}
	if len(p.config.Services) <= 1 {
		return fmt.Errorf("至少需要保留一个服务")
	}

	candidate := p.config.Clone()
	delete(candidate.Services, serviceID)
	if err := p.saveConfigLocked(candidate); err != nil {
		return err
	}

	newServices := cloneServiceProxies(p.services)
	delete(newServices, serviceID)
	p.config = candidate
	p.services = newServices
	log.Printf("服务[%s] 已删除", serviceID)
	return nil
}

// GetConfig 获取当前配置的深拷贝。
func (p *Proxy) GetConfig() *config.Config {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.config.Clone()
}

// UpdateConfig 更新配置并重建代理
func (p *Proxy) UpdateConfig(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("代理配置不能为空")
	}
	candidate := cfg.Clone()

	newServices := make(map[string]*ServiceProxy, len(candidate.Services))
	for serviceID, svcCfg := range candidate.Services {
		if svcCfg == nil {
			return fmt.Errorf("服务[%s]配置不能为空", serviceID)
		}
		serviceProxy := &ServiceProxy{}
		var err error
		serviceProxy.BlueProxy, err = createProxy(svcCfg.BlueTarget)
		if err != nil {
			return fmt.Errorf("创建服务[%s]蓝色代理失败: %v", serviceID, err)
		}
		serviceProxy.GreenProxy, err = createProxy(svcCfg.GreenTarget)
		if err != nil {
			return fmt.Errorf("创建服务[%s]绿色代理失败: %v", serviceID, err)
		}
		newServices[serviceID] = serviceProxy
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.saveConfigLocked(candidate); err != nil {
		return err
	}
	p.config = candidate
	p.services = newServices
	for serviceID, svcCfg := range candidate.Services {
		log.Printf("服务[%s](%s) 代理已重建", serviceID, svcCfg.Name)
	}
	return nil
}

func (p *Proxy) saveConfigLocked(cfg *config.Config) error {
	save := p.saveConfig
	if save == nil {
		save = config.SaveConfig
	}
	if err := save(cfg); err != nil {
		return fmt.Errorf("保存代理配置失败: %v", err)
	}
	return nil
}

func cloneServiceProxies(source map[string]*ServiceProxy) map[string]*ServiceProxy {
	cloned := make(map[string]*ServiceProxy, len(source))
	for id, serviceProxy := range source {
		cloned[id] = serviceProxy
	}
	return cloned
}

func isValidEnvironment(env string) bool {
	return env == "blue" || env == "green"
}
