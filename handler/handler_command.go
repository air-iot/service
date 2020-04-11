package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	idb "github.com/air-iot/service/db/mongo"
	"github.com/air-iot/service/logger"
	imqtt "github.com/air-iot/service/mq/mqtt"
	"github.com/air-iot/service/restful-api"
	"github.com/air-iot/service/tools"
)

var eventExecCmdLog = map[string]interface{}{"name": "执行指令事件触发"}

func TriggerExecCmd(data map[string]interface{}) error {
	//logger.Debugf(eventExecCmdLog, "开始执行执行指令事件触发器")
	//logger.Debugf(eventExecCmdLog, "传入参数为:%+v", data)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(30)*time.Second)
	defer cancel()

	nodeID, ok := data["nodeId"].(string)
	if !ok {
		return fmt.Errorf("数据消息中nodeId字段不存在或类型错误")
	}

	modelID, ok := data["modelId"].(string)
	if !ok {
		return fmt.Errorf("数据消息中modelId字段不存在或类型错误")
	}

	command, ok := data["command"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("数据消息中command字段不存在或类型错误")
	}

	commandName, ok := command["name"].(string)
	if !ok {
		return fmt.Errorf("数据消息中command对象中name字段不存在或类型错误")
	}

	data["commandName"] = commandName

	nodeObjectID, err := primitive.ObjectIDFromHex(nodeID)
	if err != nil {
		return fmt.Errorf("数据消息中nodeId转ObjectID失败")
	}

	//modelObjectID, err := primitive.ObjectIDFromHex(modelID)
	//if err != nil {
	//	return fmt.Errorf("数据消息中modelId转ObjectID失败")
	//}

	//logger.Debugf(eventExecCmdLog, "开始获取当前模型的执行指令逻辑事件")
	//获取当前模型的执行指令逻辑事件=============================================
	paramMatch := bson.D{
		bson.E{
			Key: "$match",
			Value: bson.M{
				"type":                  ExecCmd,
				"settings.node.id":      nodeID,
				"settings.command.name": commandName,
			},
		},
	}
	paramLookup := bson.D{
		bson.E{
			Key: "$lookup",
			Value: bson.M{
				"from":         "eventhandler",
				"localField":   "_id",
				"foreignField": "event",
				"as":           "handlers",
			},
		},
	}


	pipeline := mongo.Pipeline{}
	pipeline = append(pipeline, paramMatch, paramLookup)
	eventInfoList := make([]bson.M, 0)
	err = restfulapi.FindPipeline(ctx, idb.Database.Collection("event"), &eventInfoList, pipeline, nil)
	if err != nil {
		return fmt.Errorf("获取当前模型(%s)的执行指令逻辑事件失败:%s", modelID, err.Error())
	}

	//logger.Debugf(eventExecCmdLog, "开始遍历事件列表")
	for _, eventInfo := range eventInfoList {
		logger.Debugf(eventExecCmdLog, "事件信息为:%+v", eventInfo)
		handlers, ok := eventInfo["handlers"].(primitive.A)
		if !ok {
			logger.Warnln(eventExecCmdLog, "handlers字段不存在或类型错误")
			continue
		}
		if len(handlers) == 0 {
			logger.Warnln(eventExecCmdLog, "handlers字段数组长度为0")
			continue
		}

		paramMatch = bson.D{
			bson.E{
				Key: "$match",
				Value: bson.M{
					"_id": nodeObjectID,
				},
			},
		}
		paramLookup = bson.D{
			bson.E{
				Key: "$lookup",
				Value: bson.M{
					"from":         "dept",
					"localField":   "department",
					"foreignField": "_id",
					"as":           "department",
				},
			},
		}
		paramLookupModel := bson.D{
			bson.E{
				Key: "$lookup",
				Value: bson.M{
					"from":         "model",
					"localField":   "model",
					"foreignField": "_id",
					"as":           "model",
				},
			},
		}
		pipeline = mongo.Pipeline{}
		pipeline = append(pipeline, paramMatch, paramLookup, paramLookupModel)
		nodeInfoList := make([]bson.M, 0)
		err = restfulapi.FindPipeline(ctx, idb.Database.Collection("node"), &nodeInfoList, pipeline, nil)
		if err != nil {
			return fmt.Errorf("获取当前资产(%s)的详情失败:%s", nodeID, err.Error())
		}

		if len(nodeInfoList) == 0 {
			return fmt.Errorf("当前查询的资产(%s)不存在", nodeID)
		}

		nodeInfo := nodeInfoList[0]

		departmentList, ok := nodeInfo["department"].(primitive.A)
		if !ok {
			logger.Warnln(eventExecCmdLog, "资产(%s)的department字段不存在或类型错误", nodeID)
			continue
		}
		departmentMList := make([]bson.M, 0)
		for _, department := range departmentList {
			if departmentMap, ok := department.(bson.M); ok {
				departmentMList = append(departmentMList, departmentMap)
			}
		}

		data["departmentName"] = tools.FormatKeyInfoList(departmentMList, "name")

		modelList, ok := nodeInfo["model"].(primitive.A)
		if !ok {
			logger.Warnln(eventExecCmdLog, "资产(%s)的mode字段不存在或类型错误", nodeID)
			continue
		}
		modelMList := make([]bson.M, 0)
		for _, model := range modelList {
			if modelMap, ok := model.(bson.M); ok {
				modelMList = append(modelMList, modelMap)
			}
		}

		data["modelName"] = tools.FormatKeyInfoList(modelMList, "name")
		data["nodeName"] = tools.FormatKeyInfo(nodeInfo, "name")
		data["nodeUid"] = tools.FormatKeyInfo(nodeInfo, "uid")
		data["time"] = tools.GetLocalTimeNow(time.Now()).Format("2006-01-02 15:04:05")

		logger.Debugf(eventExecCmdLog, "开始分析事件")
		if eventID, ok := eventInfo["id"].(primitive.ObjectID); ok {
			//if settings, ok := eventInfo["settings"].(primitive.M); ok {
			//	if command, ok := settings["command"].(primitive.M); ok {
			//		if name, ok := command["name"].(string); ok {
			//			if commandName == name {

			sendMap := map[string]interface{}{
				"data": data,
			}
			b, err := json.Marshal(sendMap)
			if err != nil {
				continue
			}
			err = imqtt.Send(fmt.Sprintf("event/%s", eventID.Hex()), b)
			if err != nil {
				logger.Warnf(eventExecCmdLog, "发送事件(%s)错误:%s", eventID.Hex(), err.Error())
			} else {
				logger.Debugf(eventExecCmdLog, "发送事件成功:%s,数据为:%+v", eventID.Hex(), sendMap)
			}
			//			}
			//		}
			//	}
			//}
		}
	}

	//logger.Debugf(eventExecCmdLog, "执行指令事件触发器执行结束")
	return nil
}
