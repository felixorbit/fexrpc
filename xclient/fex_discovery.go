package xclient

import (
	"log"
	"net/http"
	"strings"
	"time"
)

// FexRegistryDiscovery 从注册中心获取服务列表并缓存
type FexRegistryDiscovery struct {
	*MultiServerDiscovery
	registry   string
	timeout    time.Duration // 客户端维护的服务列表过期时间
	lastUpdate time.Time     // 最后从注册中心更新的时间
}

const defaultUpdateTimeout = time.Second * 10

func NewFexRegistryDiscovery(registryAddr string, timeout time.Duration) *FexRegistryDiscovery {
	if timeout == 0 {
		timeout = defaultUpdateTimeout
	}
	return &FexRegistryDiscovery{
		MultiServerDiscovery: NewMultiServerDiscovery(make([]string, 0)),
		registry:             registryAddr,
		timeout:              timeout,
	}
}

func (d *FexRegistryDiscovery) Update(servers []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.servers = servers
	d.lastUpdate = time.Now()
	return nil
}

func (d *FexRegistryDiscovery) Refresh() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.lastUpdate.Add(d.timeout).After(time.Now()) {
		return nil
	}
	log.Println("rpc registry: refresh servers from registry", d.registry)
	resp, err := http.Get(d.registry)
	if err != nil {
		log.Println("rpc registry: refresh err: ", err)
		return err
	}
	servers := strings.Split(resp.Header.Get("X-Fexrpc-Servers"), ",")
	d.servers = make([]string, 0)
	for _, server := range servers {
		if strings.TrimSpace(server) != "" {
			d.servers = append(d.servers, server)
		}
	}
	d.lastUpdate = time.Now()
	return nil
}

func (d *FexRegistryDiscovery) Get(mode SelectMode) (string, error) {
	if err := d.Refresh(); err != nil {
		return "", err
	}
	return d.MultiServerDiscovery.Get(mode)
}

func (d *FexRegistryDiscovery) GetAll() ([]string, error) {
	if err := d.Refresh(); err != nil {
		return nil, err
	}
	return d.MultiServerDiscovery.GetAll()
}
