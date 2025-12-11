package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"ruoyi-proxy/internal/config"
	"ruoyi-proxy/internal/proxy"
)

// Handler HTTP 处理器
type Handler struct {
	proxy *proxy.Proxy
}

// New 创建处理器
func New(p *proxy.Proxy) *Handler {
	return &Handler{proxy: p}
}

// HandleStatus 管理接口 - 获取所有服务状态
func (h *Handler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	cfg := h.proxy.GetConfig()

	// 支持查询单个服务: /status?service=admin
	serviceID := r.URL.Query().Get("service")

	if serviceID != "" {
		// 查询单个服务
		svc := cfg.GetService(serviceID)
		if svc == nil {
			http.Error(w, fmt.Sprintf("服务不存在: %s", serviceID), http.StatusNotFound)
			return
		}

		status := map[string]any{
			"service_id":   serviceID,
			"name":         svc.Name,
			"active_env":   svc.ActiveEnv,
			"blue_target":  svc.BlueTarget,
			"green_target": svc.GreenTarget,
			"time":         time.Now().Format("2006-01-02 15:04:05"),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
		return
	}

	// 查询所有服务
	services := make([]map[string]any, 0, len(cfg.Services))
	for id, svc := range cfg.Services {
		services = append(services, map[string]any{
			"id":           id,
			"name":         svc.Name,
			"active_env":   svc.ActiveEnv,
			"blue_target":  svc.BlueTarget,
			"green_target": svc.GreenTarget,
			"jar_file":     svc.JarFile,
			"app_name":     svc.AppName,
		})
	}

	// 排序保证输出顺序一致
	sort.Slice(services, func(i, j int) bool {
		return services[i]["id"].(string) < services[j]["id"].(string)
	})

	status := map[string]any{
		"status":        "running",
		"service_count": len(cfg.Services),
		"services":      services,
		"proxy_port":    config.ProxyPort,
		"mgmt_port":     config.MgmtPort,
		"time":          time.Now().Format("2006-01-02 15:04:05"),
		"version":       "2.0.0",
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	json.NewEncoder(w).Encode(status)
}

// HandleSwitch 管理接口 - 切换环境
// 支持: POST /switch?service=admin&env=green  切换单个服务
// 支持: POST /switch?env=green                切换所有服务
func (h *Handler) HandleSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只支持POST方法", http.StatusMethodNotAllowed)
		return
	}

	env := r.URL.Query().Get("env")
	if env != "blue" && env != "green" {
		http.Error(w, "env参数必须是blue或green", http.StatusBadRequest)
		return
	}

	serviceID := r.URL.Query().Get("service")
	cfg := h.proxy.GetConfig()

	if serviceID != "" {
		// 切换单个服务
		svc := cfg.GetService(serviceID)
		if svc == nil {
			http.Error(w, fmt.Sprintf("服务不存在: %s", serviceID), http.StatusNotFound)
			return
		}

		oldEnv := svc.ActiveEnv
		if err := h.proxy.SwitchService(serviceID, env); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		result := map[string]any{
			"success":    true,
			"message":    fmt.Sprintf("服务[%s]已从 %s 切换到 %s", serviceID, oldEnv, env),
			"service_id": serviceID,
			"old_env":    oldEnv,
			"new_env":    env,
			"time":       time.Now().Format("2006-01-02 15:04:05"),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
		log.Printf("服务[%s]环境已从 %s 切换到 %s", serviceID, oldEnv, env)
		return
	}

	// 切换所有服务
	if err := h.proxy.SwitchAll(env); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	result := map[string]any{
		"success": true,
		"message": fmt.Sprintf("所有服务(%d个)已切换到 %s", len(cfg.Services), env),
		"new_env": env,
		"time":    time.Now().Format("2006-01-02 15:04:05"),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
	log.Printf("所有服务(%d个)环境已切换到 %s", len(cfg.Services), env)
}

// HandleUpdateConfig 管理接口 - 更新配置
func (h *Handler) HandleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只支持POST方法", http.StatusMethodNotAllowed)
		return
	}

	var newConfig config.Config
	if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
		http.Error(w, "JSON格式错误", http.StatusBadRequest)
		return
	}

	// 验证配置
	if len(newConfig.Services) == 0 {
		http.Error(w, "至少需要配置一个服务", http.StatusBadRequest)
		return
	}

	for id, svc := range newConfig.Services {
		if svc.BlueTarget == "" || svc.GreenTarget == "" {
			http.Error(w, fmt.Sprintf("服务[%s]的blue_target和green_target不能为空", id), http.StatusBadRequest)
			return
		}
		if svc.ActiveEnv != "blue" && svc.ActiveEnv != "green" {
			http.Error(w, fmt.Sprintf("服务[%s]的active_env必须是blue或green", id), http.StatusBadRequest)
			return
		}
	}

	if err := h.proxy.UpdateConfig(&newConfig); err != nil {
		log.Printf("更新配置失败: %v", err)
		http.Error(w, "更新配置失败", http.StatusInternalServerError)
		return
	}

	result := map[string]any{
		"success":       true,
		"message":       "配置更新成功",
		"service_count": len(newConfig.Services),
		"time":          time.Now().Format("2006-01-02 15:04:05"),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
	log.Printf("配置已更新，服务数: %d", len(newConfig.Services))
}

// HandleServices 管理接口 - 获取服务列表
func (h *Handler) HandleServices(w http.ResponseWriter, r *http.Request) {
	cfg := h.proxy.GetConfig()

	services := make([]string, 0, len(cfg.Services))
	for id := range cfg.Services {
		services = append(services, id)
	}
	sort.Strings(services)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"services": services,
		"count":    len(services),
	})
}

// HandleHealth 健康检查
func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, "OK")
}

// HandleServiceHealth 检查指定服务的健康状态
func (h *Handler) HandleServiceHealth(w http.ResponseWriter, r *http.Request) {
	cfg := h.proxy.GetConfig()
	serviceID := strings.TrimPrefix(r.URL.Path, "/health/")

	if serviceID == "" {
		// 检查所有服务
		results := make(map[string]map[string]any)
		client := &http.Client{Timeout: 3 * time.Second}

		for id, svc := range cfg.Services {
			target := svc.BlueTarget
			if svc.ActiveEnv == "green" {
				target = svc.GreenTarget
			}

			status := "healthy"
			if _, err := client.Get(target + "/actuator/health"); err != nil {
				status = "unhealthy"
			}

			results[id] = map[string]any{
				"name":       svc.Name,
				"active_env": svc.ActiveEnv,
				"target":     target,
				"status":     status,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
		return
	}

	// 检查单个服务
	svc := cfg.GetService(serviceID)
	if svc == nil {
		http.Error(w, fmt.Sprintf("服务不存在: %s", serviceID), http.StatusNotFound)
		return
	}

	target := svc.BlueTarget
	if svc.ActiveEnv == "green" {
		target = svc.GreenTarget
	}

	client := &http.Client{Timeout: 3 * time.Second}
	status := "healthy"
	if _, err := client.Get(target + "/actuator/health"); err != nil {
		status = "unhealthy"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"service_id": serviceID,
		"name":       svc.Name,
		"active_env": svc.ActiveEnv,
		"target":     target,
		"status":     status,
	})
}

// HandleAddService 添加新服务
func (h *Handler) HandleAddService(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只支持POST方法", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		BlueTarget  string `json:"blue_target"`
		GreenTarget string `json:"green_target"`
		JarFile     string `json:"jar_file"`
		AppName     string `json:"app_name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "JSON格式错误", http.StatusBadRequest)
		return
	}

	if req.ID == "" || req.BlueTarget == "" || req.GreenTarget == "" {
		http.Error(w, "id, blue_target, green_target不能为空", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		req.Name = req.ID
	}

	// 默认值
	if req.JarFile == "" {
		req.JarFile = "ruoyi-*.jar"
	}
	if req.AppName == "" {
		req.AppName = req.ID
	}

	// 添加服务
	svcConfig := &config.ServiceConfig{
		Name:        req.Name,
		BlueTarget:  req.BlueTarget,
		GreenTarget: req.GreenTarget,
		ActiveEnv:   "blue",
		JarFile:     req.JarFile,
		AppName:     req.AppName,
	}

	if err := h.proxy.AddService(req.ID, svcConfig); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success":    true,
		"message":    fmt.Sprintf("服务[%s]添加成功", req.ID),
		"service_id": req.ID,
	})
	log.Printf("服务[%s](%s)已添加 - JAR:%s, APP:%s", req.ID, req.Name, req.JarFile, req.AppName)
}

// HandleRemoveService 删除服务
func (h *Handler) HandleRemoveService(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "只支持DELETE方法", http.StatusMethodNotAllowed)
		return
	}

	serviceID := r.URL.Query().Get("id")
	if serviceID == "" {
		http.Error(w, "id参数不能为空", http.StatusBadRequest)
		return
	}

	if err := h.proxy.RemoveService(serviceID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success":    true,
		"message":    fmt.Sprintf("服务[%s]已删除", serviceID),
		"service_id": serviceID,
	})
	log.Printf("服务[%s]已删除", serviceID)
}
