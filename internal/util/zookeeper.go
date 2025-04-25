package util

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/samuel/go-zookeeper/zk"
)

type ZkClient struct {
	conn   *zk.Conn
	zkAddr []string
	mutex  sync.Mutex
}

func NewZkClient(zkAddr []string) (*ZkClient, error) {
	conn, _, err := zk.Connect(zkAddr, time.Second*5)
	if err != nil {
		return nil, err
	}
	return &ZkClient{conn: conn, zkAddr: zkAddr}, nil
}

func (z *ZkClient) Register(path string, data string, ttlSec int) error {
	z.mutex.Lock()
	defer z.mutex.Unlock()
	// 创建父节点
	parts := strings.Split(path, "/")
	cur := ""
	for _, p := range parts[1:len(parts)-1] {
		cur += "/" + p
		exists, _, err := z.conn.Exists(cur)
		if err != nil {
			return err
		}
		if !exists {
			_, err := z.conn.Create(cur, []byte{}, 0, zk.WorldACL(zk.PermAll))
			if err != nil && err != zk.ErrNodeExists {
				return err
			}
		}
	}
	// 注册临时节点（如果已存在则更新数据）
	exists, _, err := z.conn.Exists(path)
	if exists {
		return z.Update(path, data)
	}
	_, err = z.conn.Create(path, []byte(data), zk.FlagEphemeral, zk.WorldACL(zk.PermAll))
	if err != nil && err != zk.ErrNodeExists {
		return err
	}
	return nil
}

func (z *ZkClient) Update(path string, data string) error {
	z.mutex.Lock()
	defer z.mutex.Unlock()
	_, stat, err := z.conn.Get(path)
	if err != nil {
		return err
	}
	_, err = z.conn.Set(path, []byte(data), stat.Version)
	return err
}

func (z *ZkClient) Close() {
	z.conn.Close()
}

// 客户端发现所有可用节点并随机返回一个
func (z *ZkClient) Discover(path string) (string, error) {
	children, _, err := z.conn.Children(path)
	if err != nil {
		return "", err
	}
	if len(children) == 0 {
		return "", fmt.Errorf("no available sandbox nodes")
	}
	// 获取所有节点的性能数据
	type nodePerf struct {
		name string
		cpu float64
		data string
	}
	var nodes []nodePerf
	for _, child := range children {
		fullPath := path + "/" + child
		data, _, err := z.conn.Get(fullPath)
		if err != nil {
			continue
		}
		// 假设data为json字符串，包含cpu字段
		cpu := 0.0
		var cpuVal float64
		var dataStr = string(data)
		if strings.HasPrefix(dataStr, "{") {
			// 简单解析json中的cpu字段
			idx := strings.Index(dataStr, "\"cpu\"")
			if idx >= 0 {
				// 解析: "cpu":12.3
				remain := dataStr[idx+5:]
				colon := strings.Index(remain, ":")
				if colon >= 0 {
					remain = remain[colon+1:]
					end := strings.IndexAny(remain, ",}")
					if end >= 0 {
						val := strings.TrimSpace(remain[:end])
						fmt.Sscanf(val, "%f", &cpuVal)
						cpu = cpuVal
					}
				}
			}
		}
		nodes = append(nodes, nodePerf{name: child, cpu: cpu, data: dataStr})
	}
	if len(nodes) == 0 {
		return "", fmt.Errorf("no available sandbox nodes with perf data")
	}
	// 按cpu升序排序
	minIdx := 0
	for i := 1; i < len(nodes); i++ {
		if nodes[i].cpu < nodes[minIdx].cpu {
			minIdx = i
		}
	}
	return nodes[minIdx].data, nil
}