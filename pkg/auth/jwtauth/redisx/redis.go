package redisx

import (
	"context"
	"fmt"
	"time"

	"github.com/air-iot/service/init/redisdb"
)

// NewStore 创建基于redis存储实例
func NewStore(db redisdb.Client, keyPrefix string) *Store {
	return &Store{Cli: db, Prefix: keyPrefix}
}

// Store redis存储
type Store struct {
	Cli    redisdb.Client
	Prefix string
}

func (s *Store) wrapperKey(key string) string {
	return fmt.Sprintf("%s%s", s.Prefix, key)
}

// Set ...
func (s *Store) Set(ctx context.Context, tokenString string, expiration time.Duration) error {
	cmd := s.Cli.Set(ctx, s.wrapperKey(tokenString), "1", expiration)
	return cmd.Err()
}

// Delete ...
func (s *Store) Delete(ctx context.Context, tokenString string) (bool, error) {
	cmd := s.Cli.Del(ctx, s.wrapperKey(tokenString))
	if err := cmd.Err(); err != nil {
		return false, err
	}
	return cmd.Val() > 0, nil
}

// Check ...
func (s *Store) Check(ctx context.Context, tokenString string) (bool, error) {
	cmd := s.Cli.Exists(ctx, s.wrapperKey(tokenString))
	if err := cmd.Err(); err != nil {
		return false, err
	}
	return cmd.Val() > 0, nil
}
