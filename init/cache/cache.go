package cache

import (
	"context"
	"fmt"

	"github.com/go-redis/redis/v8"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/air-iot/service/init/mongodb"
	"github.com/air-iot/service/init/redisdb"
	"github.com/air-iot/service/util/json"
)

// Get 根据项目ID和资产ID查询数据
func Get(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, tableName, id string, result interface{}) (err error) {
	department, err := GetByDB(ctx, redisClient, mongoClient, project, tableName, id)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(department), result); err != nil {
		return err
	}
	return nil
}

// Get 根据项目ID和资产ID查询数据
func GetAll(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, tableName string, result interface{}) (err error) {
	department, err := GetByDBAll(ctx, redisClient, mongoClient, project, tableName)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(department), result); err != nil {
		return err
	}
	return nil
}

// Get 根据项目ID和资产ID查询数据
func GetAllToOne(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, tableName string, result interface{}) (err error) {
	department, err := GetByDBAllToOne(ctx, redisClient, mongoClient, project, tableName)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(department), result); err != nil {
		return err
	}
	return nil
}

func GetByDB(ctx context.Context, cli redisdb.Client, mongoClient *mongo.Client, project, tableName, id string) (string, error) {
	model, err := cli.HGet(ctx, fmt.Sprintf("%s/%s", project, tableName), id).Result()
	if err != nil && err != redis.Nil {
		return "", err
	} else if err == redis.Nil {
		col := mongoClient.Database(project).Collection(tableName)
		modelTmp := make(map[string]interface{})
		err := mongodb.FindByID(ctx, col, &modelTmp, id)
		if err != nil && err != mongo.ErrNoDocuments {
			return "", err
		}
		modelBytes, err := json.Marshal(&modelTmp)
		if err != nil {
			return "", err
		}
		err = Update(ctx, cli, project,tableName, id, modelTmp)
		if err != nil {
			return "", fmt.Errorf("更新缓存数据错误, %v", err)
		}
		model = string(modelBytes)
	}
	return model, nil
}

func GetByDBAll(ctx context.Context, cli redisdb.Client, mongoClient *mongo.Client, project, tableName string) (string, error) {
	modelReturn := ""
	model, err := cli.HGetAll(ctx, fmt.Sprintf("%s/%s", project, tableName)).Result()
	if err != nil && err != redis.Nil {
		return "", err
	} else if err == redis.Nil {
		col := mongoClient.Database(project).Collection(tableName)
		modelTmp := make([]map[string]interface{},0)
		err := mongodb.FindPipeline(ctx, col, &modelTmp, mongo.Pipeline{})
		if err != nil {
			return "", err
		}
		modelBytes, err := json.Marshal(&modelTmp)
		if err != nil {
			return "", err
		}
		for _,ele := range modelTmp {
			if id,ok := ele["id"].(string);ok{
				eleBytes, err := json.Marshal(&ele)
				if err != nil {
					return "", err
				}
				err = Set(ctx, cli, project,tableName, id, eleBytes)
				if err != nil {
					return "", fmt.Errorf("更新缓存数据错误, %v", err)
				}
			}
		}
		modelReturn = string(modelBytes)
	}else{
		modelTmp := make([]map[string]interface{},0)
		eleTmp := map[string]interface{}{}
		for _,ele :=  range model{
			err := json.Unmarshal([]byte(ele),&eleTmp)
			if err != nil {
				return "", err
			}
			modelTmp = append(modelTmp,eleTmp)
		}
		eleBytes, err := json.Marshal(&modelTmp)
		if err != nil {
			return "", err
		}
		modelReturn = string(eleBytes)
	}
	return modelReturn, nil
}

func GetByDBAllToOne(ctx context.Context, cli redisdb.Client, mongoClient *mongo.Client, project, tableName string) (string, error) {
	modelReturn := ""
	model, err := cli.HGetAll(ctx, fmt.Sprintf("%s/%s", project, tableName)).Result()
	if err != nil && err != redis.Nil {
		return "", err
	} else if err == redis.Nil {
		col := mongoClient.Database(project).Collection(tableName)
		modelTmp := make([]map[string]interface{},0)
		err := mongodb.FindPipeline(ctx, col, &modelTmp, mongo.Pipeline{})
		if err != nil {
			return "", err
		}
		modelTmpOne := map[string]interface{}{}
		if len(modelTmp) != 0 {
			modelTmpOne = modelTmp[0]
		}
		modelBytes, err := json.Marshal(&modelTmpOne)
		if err != nil {
			return "", err
		}
		for _,ele := range modelTmp {
			if id,ok := ele["id"].(string);ok{
				eleBytes, err := json.Marshal(&ele)
				if err != nil {
					return "", err
				}
				err = Set(ctx, cli, project,tableName, id, eleBytes)
				if err != nil {
					return "", fmt.Errorf("更新缓存数据错误, %v", err)
				}
			}
		}
		modelReturn = string(modelBytes)
	}else{
		modelTmp := make([]map[string]interface{},0)
		eleTmp := map[string]interface{}{}
		for _,ele :=  range model{
			err := json.Unmarshal([]byte(ele),&eleTmp)
			if err != nil {
				return "", err
			}
			modelTmp = append(modelTmp,eleTmp)
		}
		modelTmpOne := map[string]interface{}{}
		if len(modelTmp) != 0 {
			modelTmpOne = modelTmp[0]
		}
		eleBytes, err := json.Marshal(&modelTmpOne)
		if err != nil {
			return "", err
		}
		modelReturn = string(eleBytes)
	}
	return modelReturn, nil
}


func Set(ctx context.Context, cli redisdb.Client, project, tableName, id string, data interface{}) error {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化数据错误, %v", err)
	}
	tx := cli.TxPipeline()
	locked, err := tx.HSetNX(ctx, fmt.Sprintf("%s/%s/lock", project, tableName), id, 0).Result()
	if err != nil {
		return fmt.Errorf("增加缓存锁错误, %v", err)
	}
	if locked {
		b, err := tx.HMSet(ctx, fmt.Sprintf("%s/%s", project, tableName), map[string]interface{}{id: dataBytes}).Result()
		if err != nil {
			return fmt.Errorf("更新缓存数据错误, %v", err)
		}
		if !b {
			return fmt.Errorf("保存数据未成功")
		}
	}
	if _, err := tx.Exec(ctx); err != nil {
		return fmt.Errorf("提交更新缓存数据事务错误, %v", err)
	}
	return nil
}

// Update 更新redis数据
func Update(ctx context.Context, cli redisdb.Client, project, tableName, id string, data interface{}) error {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化数据错误, %v", err)
	}
	tx := cli.TxPipeline()
	_, err = tx.HSetNX(ctx, fmt.Sprintf("%s/%s/lock", project, tableName), id, 0).Result()
	if err != nil {
		return fmt.Errorf("增加缓存数据锁错误, %v", err)
	}
	b, err := tx.HMSet(ctx, fmt.Sprintf("%s/%s", project, tableName), map[string]interface{}{id: dataBytes}).Result()
	if err != nil {
		return fmt.Errorf("更新缓存数据错误, %v", err)
	}
	_ = b
	//if !b {
	//	return fmt.Errorf("保存数据未成功")
	//}
	if _, err := tx.Exec(ctx); err != nil {
		return fmt.Errorf("提交更新缓存数据事务错误, %v", err)
	}
	return nil
}

// Delete 删除redis数据
func Delete(ctx context.Context, cli redisdb.Client, project, tableName, id string) error {
	tx := cli.TxPipeline()
	_, err := tx.HDel(ctx, fmt.Sprintf("%s/%s/lock", project, tableName), id).Result()
	if err != nil {
		return fmt.Errorf("删除缓存数据锁错误, %v", err)
	}
	_, err = tx.HDel(ctx, fmt.Sprintf("%s/%s", project, tableName), id).Result()
	if err != nil {
		return fmt.Errorf("删除缓存数据错误, %v", err)
	}
	if _, err := tx.Exec(ctx); err != nil {
		return fmt.Errorf("提交删除缓存数据事务错误, %v", err)
	}
	return nil
}
