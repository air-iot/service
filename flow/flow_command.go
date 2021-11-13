package flow

import (
	"context"
	"fmt"
	"github.com/air-iot/service/init/cache/flow"
	"github.com/air-iot/service/util/flowx"
	"github.com/camunda-cloud/zeebe/clients/go/pkg/zbc"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/air-iot/service/api"
	"github.com/air-iot/service/gin/ginx"
	"github.com/air-iot/service/init/cache/entity"
	"github.com/air-iot/service/init/cache/model"
	"github.com/air-iot/service/init/cache/node"
	"github.com/air-iot/service/init/mq"
	"github.com/air-iot/service/init/redisdb"
	"github.com/air-iot/service/logger"
	"github.com/air-iot/service/util/formatx"
	"github.com/air-iot/service/util/timex"
)

func TriggerExecCmdFlow(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, mq mq.MQ, apiClient api.Client, zbClient zbc.Client, projectName string,
	data map[string]interface{}) error {

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

	cmdStatus, ok := data["cmdStatus"].(string)
	if !ok {
		return fmt.Errorf("数据消息中cmdStatus字段不存在或类型错误")
	}

	headerMap := map[string]string{ginx.XRequestProject: projectName}
	flowInfoList := new([]entity.Flow)
	//logger.Debugf(flowExecCmdLog, "开始获取当前模型的执行指令逻辑流程")
	//获取当前模型的执行指令逻辑流程=============================================
	err := flow.GetByType(ctx, redisClient, mongoClient, projectName, string(ExecCmd), flowInfoList)
	if err != nil {
		return fmt.Errorf("获取执行指令流程失败: %s", err.Error())
	}
	nodeInfo := make(map[string]interface{})
	err = node.Get(ctx, redisClient, mongoClient, projectName, nodeID, &nodeInfo)
	if err != nil {
		return fmt.Errorf("获取当前模型(%s)的资产(%s)失败:%s", modelID, nodeID, err.Error())
	}

	modelInfo := make(map[string]interface{})
	err = model.Get(ctx, redisClient, mongoClient, projectName, modelID, &modelInfo)
	if err != nil {
		return fmt.Errorf("获取当前模型(%s)详情失败:%s", modelID, err.Error())
	}

	departmentStringIDList := make([]string, 0)
	//var departmentObjectList primitive.A
	if departmentIDList, ok := nodeInfo["department"].([]interface{}); ok {
		departmentStringIDList = formatx.InterfaceListToStringList(departmentIDList)
	} else {
		logger.Warnf("资产(%s)的部门字段不存在或类型错误", nodeID)
	}

	//deptInfoList := make([]map[string]interface{}, 0)
	//if len(departmentStringIDList) != 0 {
	//	deptInfoList, err = clogic.DeptLogic.FindLocalCacheList(departmentStringIDList)
	//	if err != nil {
	//		return fmt.Errorf("获取当前资产(%s)所属部门失败:%s", nodeID, err.Error())
	//	}
	//}

	//logger.Debugf(flowExecCmdLog, "开始遍历流程列表")
	for _, flowInfo := range *flowInfoList {
		flowID := flowInfo.ID
		settings := flowInfo.Settings

		//判断是否已经失效
		if flowInfo.Invalid {
			logger.Warnln("流程(%s)已经失效", flowID)
			continue
		}

		//判断禁用
		if flowInfo.Disable {
			logger.Warnln("流程(%s)已经被禁用", flowID)
			continue
		}

		//if flowInfo.ValidTime == "timeLimit" {
		//	if flowInfo.Range != "once" {
		//判断有效期
		startTime := flowInfo.StartTime
		formatLayout := timex.FormatTimeFormat(startTime)
		if startTime != "" {
			formatStartTime, err := timex.ConvertStringToTime(formatLayout, startTime, time.Local)
			if err != nil {
				logger.Errorf("开始时间范围字段值格式错误:%s", err.Error())
				continue
			}
			if timex.GetLocalTimeNow(time.Now()).Unix() < formatStartTime.Unix() {
				logger.Debugf("流程(%s)的定时任务开始时间未到，不执行", flowID)
				continue
			}
		}

		endTime := flowInfo.EndTime
		formatLayout = timex.FormatTimeFormat(endTime)
		if endTime != "" {
			formatEndTime, err := timex.ConvertStringToTime(formatLayout, endTime, time.Local)
			if err != nil {
				logger.Errorf("时间范围字段值格式错误:%s", err.Error())
				continue
			}
			if timex.GetLocalTimeNow(time.Now()).Unix() >= formatEndTime.Unix() {
				logger.Debugf("流程(%s)的定时任务结束时间已到，不执行", flowID)
				//修改流程为失效
				updateMap := bson.M{"invalid": true}
				//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("flow"), eventID, updateMap)
				var r= make(map[string]interface{})
				err := apiClient.UpdateFlowById(headerMap, flowID, updateMap, &r)
				if err != nil {
					logger.Errorf("失效流程(%s)失败:%s", flowID, err.Error())
					continue
				}
				continue
			}
		}
		//	}
		//}

		//判断流程是否已经触发
		hasExecute := false
		logger.Debugf("开始分析流程")
		isValidCmd := false
		//判断该流程是否指定了特定资产
		if nodeMap, ok := settings["node"].(map[string]interface{}); ok {
			if nodeIDInSettings, ok := nodeMap["id"].(string); ok {
				if nodeID == nodeIDInSettings {
					isValidCmd = true
				}
			}
		} else if modelMap, ok := settings["model"].(map[string]interface{}); ok {
			if modelIDInSettings, ok := modelMap["id"].(string); ok {
				if modelID == modelIDInSettings {
					isValidCmd = true
				}
			}
		}

		if !isValidCmd {
			continue
		}

		isValidStatus := false
		if cmdStatusInSetting, ok := settings["cmdStatus"].(string); ok {
			if cmdStatus == cmdStatusInSetting {
				isValidStatus = true
			}
		}

		if !isValidStatus {
			continue
		}

		if commandList, ok := settings["command"].([]interface{}); ok {
			for _, commandRaw := range commandList {
				if command, ok := commandRaw.(map[string]interface{}); ok {
					if name, ok := command["name"].(string); ok {
						if commandName == name {
							deptMap := bson.M{}
							for _, id := range departmentStringIDList {
								deptMap[id] = bson.M{"id": id, "_tableName": "dept"}
							}
							sendMap := bson.M{
								"time":         timex.GetLocalTimeNow(time.Now()).Format("2006-01-02 15:04:05"),
								"#$model":      bson.M{"id": modelID, "_tableName": "model"},
								"#$department": deptMap,
								"#$node":       bson.M{"id": nodeID, "_tableName": "node", "uid": nodeID},
								"cmdName":      commandName,
								//"departmentName": departmentStringIDList,
								//"modelName":      formatx.FormatKeyInfo(modelInfo, "name"),
								//"nodeName":       formatx.FormatKeyInfo(nodeInfo, "name"),
							}

							err = flowx.StartFlow(mongoClient,zbClient, flowInfo.FlowXml, projectName, string(CommandTrigger),flowID,sendMap,settings)
							if err != nil {
								logger.Errorf("流程推进到下一阶段失败:%s", err.Error())
								break
							}

							hasExecute = true
							break
						}
					}
				}
			}
		}

		//对只能执行一次的流程进行失效
		if flowInfo.ValidTime == "timeLimit" {
			if flowInfo.Range == "once" && hasExecute {
				logger.Warnln("流程(%s)为只执行一次的流程", flowID)
				//修改流程为失效
				updateMap := bson.M{"invalid": true}
				//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
				var r = make(map[string]interface{})
				err := apiClient.UpdateFlowById(headerMap, flowID, updateMap, &r)
				if err != nil {
					logger.Errorf("失效流程(%s)失败:%s", flowID, err.Error())
					continue
				}
			}
		}
	}

	//logger.Debugf(eventExecCmdLog, "执行指令流程触发器执行结束")
	return nil
}
