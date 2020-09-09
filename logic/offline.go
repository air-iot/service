package logic

import (
	"errors"
	"sync"
)

var OfflineStatusLogic = new(offlineStatusLogic)

type offlineStatusLogic struct {
	offlineStatusCache *sync.Map
}

func (p *offlineStatusLogic) FindLocalCacheList(nodeIDs []string) (result map[string]interface{}, err error) {
	result = map[string]interface{}{}
	for _, nodeID := range nodeIDs {
		a, b := p.offlineStatusCache.Load(nodeID)
		if b {
			result[nodeID] = a
		} else {
			return nil, errors.New("未查询到相关数据")
		}
	}
	return result, nil
}

func (p *offlineStatusLogic) FindLocalCache(nodeID string) (result interface{}, err error) {
	a, b := p.offlineStatusCache.Load(nodeID)
	if b {
		return a, nil
	} else {
		return nil, errors.New("未查询到相关数据")
	}
}
