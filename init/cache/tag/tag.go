package tag

import (
	"context"
	"fmt"
	"github.com/air-iot/service/init/cache/entity"
	"github.com/air-iot/service/init/cache/model"
	"github.com/air-iot/service/init/cache/node"
	"github.com/go-redis/redis/v8"
	"go.mongodb.org/mongo-driver/mongo"
)


// 根据模型id、节点id与数据点id查询数据点信息
func FindLocalCache(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, modelID, nodeID, tagID string) (*entity.Tag, error) {
	//cacheID := fmt.Sprintf("%s|%s", nodeID, tagID)
	//tag1, ok := p.tagCache.Load(cacheID)
	//if ok {
	//	t, ok := tag1.(model.Tag)
	//	if ok {
	//		return &t, err
	//	}
	//}
	var modelAuto = false
	// 查询模型tag
	modelInfo := entity.Model{}
	if err := model.Get(ctx, redisClient, mongoClient, project, modelID, &modelInfo); err == nil {
		if t, err := tagModel(&modelInfo, tagID); err == nil {
			//p.tagCache.Store(cacheID, t)
			return t, err
		}
		modelAuto = modelInfo.Computed.Auto
	}

	nodeInfo := entity.Node{}
	if err := node.Get(ctx, redisClient, mongoClient, project, nodeID, &nodeInfo); err == nil {
		if t, err := tagNode(&nodeInfo, tagID); err == nil {
			//p.tagCache.Store(cacheID, t)
			return t, err
		}

		if modelAuto {
			// 查询节点子节点
			for _, c := range nodeInfo.Child {
				// 查询子节点
				r := entity.Node{}
				if err := 	node.Get(ctx, redisClient, mongoClient, project, c, &r); err == nil {
					if t, err := tagNode(&r, tagID); err == nil {
						//p.tagCache.Store(cacheID, t)
						return t, err
					}
					// 查询子节点模型
					m := entity.Model{}
					if err := model.Get(ctx, redisClient, mongoClient, project, r.Model, &m); err == nil {
						if t, err := tagModel(&m, tagID); err == nil {
							//tagCache.Store(cacheID, t)
							return t, err
						}
					}
				}
			}
		}
	}

	return nil, fmt.Errorf("模型:%s 节点:%s 中未找到tag:%s", modelID, nodeID, tagID)
}

func tagModel(result *entity.Model, tagID string) (*entity.Tag, error) {
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

func tagNode(result *entity.Node, tagID string) (*entity.Tag, error) {
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
