package cache

// CacheMethod 通过mq处理缓存的方式
type CacheMethod string

const (
	Update CacheMethod = "update"
	Delete CacheMethod = "delete"
)

// CacheParams 缓存配置
type CacheParams struct {
	Exchange, Queue string
}
