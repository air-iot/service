package redis

import (
	"context"
	"net"
	"strconv"
	"time"

	//"github.com/go-redis/redis"
	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var Client *redis.Client
var ClusterClient *redis.ClusterClient
var PoolSize int
var ClusterBool = false

func Init() {
	if !viper.GetBool("redis.enable") {
		return
	}
	var (
		host     = viper.GetString("redis.host")
		port     = viper.GetInt("redis.port")
		username = viper.GetString("redis.username")
		password = viper.GetString("redis.password")
		db       = viper.GetInt("redis.db")
	)
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	PoolSize = viper.GetInt("redis.poolSize")
	ClusterBool = viper.GetBool("redis.cluster")
	if ClusterBool {
		clusterNodes := make([]redis.ClusterNode, 0)
		clusterNode := viper.GetStringSlice("redis.clusterNode")
		for _, n := range clusterNode {
			clusterNodes = append(clusterNodes, redis.ClusterNode{
				Addr: n,
			})
		}

		ClusterClient = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:    []string{addr},
			Username: username,
			Password: password, // no password set
			ClusterSlots: func(ctx context.Context) ([]redis.ClusterSlot, error) {
				return []redis.ClusterSlot{
					{
						Nodes: clusterNodes,
					},
				}, nil
			},
		})
		p := ClusterClient.Ping(context.Background())
		if p.Err() != nil {
			panic(p.Err())
		}
		return
	}
	Client = redis.NewClient(&redis.Options{
		Addr:     addr,
		Username: username,
		Password: password, // no password set
		DB:       db,       // use default DB
		PoolSize: PoolSize,
		//MinIdleConns: 10,
		//PoolTimeout:10*time.Second
		//IdleTimeout:  2 * time.Second,
		DialTimeout:  10 * time.Second,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	})
	p := Client.Ping(context.Background())
	if p.Err() != nil {
		panic(p.Err())
	}
}

func Close() {
	if Client != nil {
		if err := Client.Close(); err != nil {
			logrus.Errorf("Redis关闭失败:%s", err.Error())
		}
	}
}

// Set 保存数据
func Set(c *redis.Client, data map[string]interface{}, expire time.Duration) error {
	p := c.TxPipeline()
	for k, v := range data {
		p.Set(context.Background(), k, v, expire)
	}
	if _, err := p.Exec(context.Background()); err != nil {
		return err
	}
	return nil
}

// Get 获取数据
func Get(c *redis.Client, keys ...string) (map[string]string, error) {
	p := c.Pipeline()

	query := make(map[string]*redis.StringCmd)
	for _, key := range keys {
		query[key] = p.Get(context.Background(), key)
	}
	if _, err := p.Exec(context.Background()); err != nil {
		return nil, err
	}
	results := make(map[string]string)
	for k, cmd := range query {
		if cmd.Err() != nil {
			return nil, cmd.Err()
		}
		results[k] = cmd.Val()
	}

	return results, nil
}

// SetMap 保存map数据
func SetMap(c *redis.Client, key string, value map[string]interface{}) error {
	if cmd := c.HMSet(context.Background(), key, value); cmd.Err() != nil {
		return cmd.Err()
	}
	return nil
}

// SetMaps 保存map数组数据到map
func SetMaps(c *redis.Client, data map[string]map[string]interface{}) error {
	p := c.TxPipeline()
	for k, v := range data {
		p.HMSet(context.Background(), k, v)
	}
	if _, err := p.Exec(context.Background()); err != nil {
		return err
	}
	return nil
}

// GetMaps 从map批量查询数据
func GetMaps(c *redis.Client, keys ...string) (map[string]map[string]string, error) {
	p := c.Pipeline()

	query := make(map[string]*redis.StringStringMapCmd)
	for _, key := range keys {
		query[key] = p.HGetAll(context.Background(), key)
	}
	if _, err := p.Exec(context.Background()); err != nil {
		return nil, err
	}
	results := make(map[string]map[string]string)
	for k, cmd := range query {
		val, err := cmd.Result()
		if err != nil {
			return nil, err
		}
		results[k] = val
	}

	return results, nil
}
