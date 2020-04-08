package logic

import (
	"fmt"
	"sync"

	"common/model"
)

var TagLogic = new(tagLogic)

type tagLogic struct {
	sync.Mutex
	tagCache *sync.Map
}

// 根据模型id、节点id与数据点id查询数据点信息
func (p *tagLogic) FindLocalCache(modelID, nodeID, uid, tagID string) (result *model.Tag, err error) {
	cacheID := fmt.Sprintf("%s|%s", uid, tagID)
	tag1, ok := p.tagCache.Load(cacheID)
	if ok {
		t, ok := tag1.(model.Tag)
		if ok {
			return &t, err
		}
	}
	var modelAuto = false
	// 查询模型tag
	if result, err := ModelLogic.FindLocalCache(modelID); err == nil {
		if t, err := p.tagModel(result, tagID); err == nil {
			p.tagCache.Store(cacheID, t)
			return t, err
		}
		modelAuto = result.Computed.Auto
	}

	if result, err := NodeLogic.FindLocalCache(nodeID); err == nil {

		if t, err := p.tagNode(result, tagID); err == nil {
			p.tagCache.Store(cacheID, t)
			return t, err
		}

		if modelAuto {
			// 查询节点子节点
			for _, c := range result.Child {
				// 查询子节点
				if r, err := NodeLogic.FindLocalCache(c); err == nil {
					if t, err := p.tagNode(r, tagID); err == nil {
						p.tagCache.Store(cacheID, t)
						return t, err
					}
					// 查询子节点模型
					if r1, err := ModelLogic.FindLocalCache(r.Model); err == nil {
						if t, err := p.tagModel(r1, tagID); err == nil {
							p.tagCache.Store(cacheID, t)
							return t, err
						}
					}
				}
			}
		}
	}

	return nil, fmt.Errorf("模型:%s 节点:%s uid:%s 中未找到tag:%s", modelID, nodeID, uid, tagID)
}

func (p *tagLogic) tagModel(result *model.Model, tagID string) (*model.Tag, error) {
	for _, t := range result.Device.Tags {
		if t.ID == tagID {
			return &t, nil
		}
	}
	for _, t := range result.Computed.Tags {
		if t.ID == tagID {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("模型中未找到%s数据点", tagID)
}

func (p *tagLogic) tagNode(result *model.Node, tagID string) (*model.Tag, error) {
	for _, t := range result.Device.Tags {
		if t.ID == tagID {
			return &t, nil
		}
	}
	for _, t := range result.Computed.Tags {
		if t.ID == tagID {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("节点中未找到%s数据点", tagID)
}
