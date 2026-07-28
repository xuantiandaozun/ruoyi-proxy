package proxy

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ruoyi-proxy/internal/config"
)

func TestGetConfigReturnsDeepCopy(t *testing.T) {
	p := newTestProxy(t, "http://127.0.0.1:18080", "http://127.0.0.1:18081")

	snapshot := p.GetConfig()
	snapshot.Services["default"].ActiveEnv = "green"
	snapshot.Services["extra"] = &config.ServiceConfig{Name: "额外服务"}

	current := p.GetConfig()
	if current.Services["default"].ActiveEnv != "blue" {
		t.Fatal("修改配置快照不应影响代理内部状态")
	}
	if _, exists := current.Services["extra"]; exists {
		t.Fatal("修改配置快照的 map 不应影响代理内部状态")
	}
}

func TestSwitchServiceRollsBackWhenSaveFails(t *testing.T) {
	p := newTestProxy(t, "http://127.0.0.1:18080", "http://127.0.0.1:18081")
	p.saveConfig = func(*config.Config) error { return errors.New("磁盘只读") }

	if err := p.SwitchService("default", "green"); err == nil {
		t.Fatal("保存失败时应返回错误")
	}
	if got := p.GetConfig().Services["default"].ActiveEnv; got != "blue" {
		t.Fatalf("保存失败后内存环境=%s，期望 blue", got)
	}
}

func TestAddAndRemoveServiceRollBackWhenSaveFails(t *testing.T) {
	p := newTestProxy(t, "http://127.0.0.1:18080", "http://127.0.0.1:18081")
	failure := errors.New("磁盘只读")
	p.saveConfig = func(*config.Config) error { return failure }
	extra := &config.ServiceConfig{
		Name:        "额外服务",
		BlueTarget:  "http://127.0.0.1:28080",
		GreenTarget: "http://127.0.0.1:28081",
		ActiveEnv:   "blue",
	}
	if err := p.AddService("extra", extra); err == nil {
		t.Fatal("保存失败时添加服务应返回错误")
	}
	if _, exists := p.GetConfig().Services["extra"]; exists {
		t.Fatal("保存失败后不应把新增服务写入内存配置")
	}
	if _, exists := p.services["extra"]; exists {
		t.Fatal("保存失败后不应把新增服务写入路由表")
	}

	p.saveConfig = func(*config.Config) error { return nil }
	if err := p.AddService("extra", extra); err != nil {
		t.Fatalf("准备额外服务失败: %v", err)
	}
	p.saveConfig = func(*config.Config) error { return failure }
	if err := p.RemoveService("extra"); err == nil {
		t.Fatal("保存失败时删除服务应返回错误")
	}
	if _, exists := p.GetConfig().Services["extra"]; !exists {
		t.Fatal("保存失败后不应从内存配置删除服务")
	}
	if _, exists := p.services["extra"]; !exists {
		t.Fatal("保存失败后不应从路由表删除服务")
	}
}

func TestUpdateConfigRollsBackWhenSaveFails(t *testing.T) {
	p := newTestProxy(t, "http://127.0.0.1:18080", "http://127.0.0.1:18081")
	candidate := p.GetConfig()
	candidate.Services["default"].Name = "新名称"
	p.saveConfig = func(*config.Config) error { return errors.New("磁盘只读") }

	if err := p.UpdateConfig(candidate); err == nil {
		t.Fatal("保存失败时更新配置应返回错误")
	}
	if got := p.GetConfig().Services["default"].Name; got != "默认服务" {
		t.Fatalf("保存失败后服务名称=%q，期望保留原值", got)
	}
}
func TestSwitchServiceRejectsInvalidEnvironment(t *testing.T) {
	p := newTestProxy(t, "http://127.0.0.1:18080", "http://127.0.0.1:18081")
	if err := p.SwitchService("default", "invalid"); err == nil {
		t.Fatal("无效环境必须被拒绝")
	}
}

func TestLongRequestDoesNotBlockEnvironmentSwitch(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()

	blue := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = w.Write([]byte("blue"))
	}))
	defer blue.Close()
	green := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("green"))
	}))
	defer green.Close()

	p := newTestProxy(t, blue.URL, green.URL)
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		p.HandleProxy(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/slow", nil))
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("上游长请求未启动")
	}

	switchDone := make(chan error, 1)
	go func() {
		switchDone <- p.SwitchService("default", "green")
	}()
	select {
	case err := <-switchDone:
		if err != nil {
			t.Fatalf("切换环境失败: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("环境切换被活动长请求阻塞")
	}

	close(release)
	released = true
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("释放上游后代理请求未结束")
	}

	recorder := httptest.NewRecorder()
	p.HandleProxy(recorder, httptest.NewRequest(http.MethodGet, "/next", nil))
	if recorder.Body.String() != "green" {
		t.Fatalf("切换后的请求返回 %q，期望 green", recorder.Body.String())
	}
}

func newTestProxy(t *testing.T, blueTarget, greenTarget string) *Proxy {
	t.Helper()
	blueProxy, err := createProxy(blueTarget)
	if err != nil {
		t.Fatalf("创建蓝色代理失败: %v", err)
	}
	greenProxy, err := createProxy(greenTarget)
	if err != nil {
		t.Fatalf("创建绿色代理失败: %v", err)
	}
	return &Proxy{
		config: &config.Config{Services: map[string]*config.ServiceConfig{
			"default": {
				Name:        "默认服务",
				BlueTarget:  blueTarget,
				GreenTarget: greenTarget,
				ActiveEnv:   "blue",
			},
		}},
		services: map[string]*ServiceProxy{
			"default": {BlueProxy: blueProxy, GreenProxy: greenProxy},
		},
		saveConfig: func(*config.Config) error { return nil },
	}
}
