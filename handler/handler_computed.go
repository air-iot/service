package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	iredis "github.com/air-iot/service/db/redis"
	"github.com/air-iot/service/logger"
	clogic "github.com/air-iot/service/logic"
	cmodel "github.com/air-iot/service/model"
	imqtt "github.com/air-iot/service/mq/mqtt"
	"github.com/air-iot/service/tools"
	"github.com/diegoholiveira/jsonlogic"
	"github.com/go-redis/redis"
	"go.mongodb.org/mongo-driver/bson"
)

var eventComputeLogicLog = map[string]interface{}{"name": "计算逻辑事件触发"}

func TriggerComputed(data cmodel.DataMessage) error {
	//logger.Debugf(eventComputeLogicLog, "开始执行计算事件触发器")
	//logger.Debugf(eventComputeLogicLog, "传入参数为:%+v", data)

	nodeID := data.NodeID
	if nodeID == "" {
		logger.Errorf(eventComputeLogicLog, fmt.Sprintf("数据消息中nodeId字段不存在或类型错误"))
		return fmt.Errorf("数据消息中nodeId字段不存在或类型错误")
	}

	modelID := data.ModelID
	if nodeID == "" {
		logger.Errorf(eventComputeLogicLog, fmt.Sprintf("数据消息中modelId字段不存在或类型错误"))
		return fmt.Errorf("数据消息中modelId字段不存在或类型错误")
	}

	inputMap := data.InputMap

	fieldsMap := data.Fields
	if len(fieldsMap) == 0 {
		logger.Errorf(eventComputeLogicLog, fmt.Sprintf("数据消息中fields字段不存在或类型错误"))
		return fmt.Errorf("数据消息中fields字段不存在或类型错误")
	}
	//logger.Debugf(eventComputeLogicLog, "开始获取当前模型的计算逻辑事件")
	//获取当前模型的计算逻辑事件=============================================
	eventInfoList, err := clogic.EventLogic.FindLocalCache(modelID + "|" + string(ComputeLogic))
	if err != nil {
		logger.Debugf(eventComputeLogicLog, fmt.Sprintf("获取当前模型(%s)的计算逻辑事件失败:%s", modelID, err.Error()))
		return fmt.Errorf("获取当前模型(%s)的计算逻辑事件失败:%s", modelID, err.Error())
	}

	nodeInfo, err := clogic.NodeLogic.FindLocalMapCache(nodeID)
	if err != nil {
		logger.Errorf(eventComputeLogicLog, fmt.Sprintf("获取当前模型(%s)的资产(%s)失败:%s", modelID, nodeID, err.Error()))
		return fmt.Errorf("获取当前模型(%s)的资产(%s)失败:%s", modelID, nodeID, err.Error())
	}

	modelInfo, err := clogic.ModelLogic.FindLocalMapCache(modelID)
	if err != nil {
		logger.Errorf(eventComputeLogicLog, fmt.Sprintf("获取当前模型(%s)详情失败:%s", modelID, err.Error()))
		return fmt.Errorf("获取当前模型(%s)详情失败:%s", modelID, err.Error())
	}

	//logger.Debugf(eventComputeLogicLog, "开始遍历事件列表")
ruleloop:
	for _, eventInfo := range *eventInfoList {
		logger.Debugf(eventComputeLogicLog, "事件信息为:%+v", eventInfo)
		if eventInfo.Handlers == nil || len(eventInfo.Handlers) == 0 {
			logger.Warnln(eventComputeLogicLog, "handlers字段数组长度为0")
			continue
		}
		logger.Debugf(eventComputeLogicLog, "开始分析事件")
		eventID := eventInfo.ID
		settings := eventInfo.Settings
		if logic, ok := settings["logic"].(map[string]interface{}); ok {
			logger.Debugf(eventComputeLogicLog, "开始分析事件的计算逻辑")
			logger.Debugf(eventComputeLogicLog, "开始针对资产进行计算")
			nodeID, ok := nodeInfo["id"].(string)
			if !ok {
				return fmt.Errorf("当前模型(%s)下的资产ID不存在或类型错误", modelID)
			}
			uid, ok := nodeInfo["uid"].(string)
			if !ok {
				return fmt.Errorf("资产(%s)的资产编号不存在或类型错误", nodeID)
			}
			logger.Debugf(eventComputeLogicLog, "资产(%s)的报警规则为:%+v", nodeID, logic)
			tagIDList := make([]string, 0)
			//=======
			nodePropList := make([]string, 0)
			//tools.GetJsonLogicVarList(logic, &tagIDList)
			tools.GetJsonLogicVarListPointVar(logic, &tagIDList)
			tools.GetJsonLogicVarListNode(logic, &nodePropList)
			logicMap := map[string]interface{}{}
			//判断传来的tag是否在该计算公式中
			isExist := false
			//判断是否存在纯数字的数据点ID
			for _, tagIDInList := range tagIDList {
				if tools.IsNumber(tagIDInList) {
					logger.Errorf(eventComputeLogicLog, "资产(%s)的报警规则中存在纯数字的数据点ID:%s", nodeID, tagIDInList)
					continue ruleloop
				}
			}
			for _, tagIDInList := range tagIDList {
				if _, ok := fieldsMap[tagIDInList]; ok {
					isExist = true
					break
				}
			}
			if !isExist {
				continue
			}
			//遍历当前tagID数组放入公式中
			//for _, tagIDInList := range tagIDList {
			//	if fieldsVal, ok := fieldsMap[tagIDInList]; ok {
			//		logicMap[tagIDInList] = fieldsVal
			//	}
			//	//if tagID == tagIDInList {
			//	//	logicMap[tagID] = tagVal
			//	//	break
			//	//}
			//}
			cmdList := make([]*redis.StringCmd, 0)
			pipe := iredis.Client.Pipeline()
			for _, tagIDInList := range tagIDList {
				//不在fieldsMap中的tagId就去查redis
				if fieldsVal, ok := fieldsMap[tagIDInList]; !ok {
					//如果公式中的该参数为输入值类型则不用查Redis，直接套用
					if inputVal, ok := inputMap[tagIDInList]; ok {
						logicMap[tagIDInList] = inputVal
						continue
					} else {
						hashKey := uid + "_" + tagIDInList
						cmd := pipe.HGet(hashKey, "value")
						cmdList = append(cmdList, cmd)
					}
				} else {
					logicMap[tagIDInList] = fieldsVal
				}
				//if tagID != tagIDInList {
				//	hashKey := nodeID + "_" + tagIDInList
				//	cmd := pipe.HGet(hashKey, "value")
				//	cmdList = append(cmdList, cmd)
				//}
			}
			_, err = pipe.Exec()
			if err != nil {
				logger.Errorf(eventComputeLogicLog, "Redis批量查询tag最新数据(指令为:%+v)失败:%s", cmdList, err.Error())
				return fmt.Errorf("Redis批量查询tag最新数据(指令为:%+v)失败:%s", cmdList, err.Error())
			}
			resultIndex := 0
			//if len(tagIDList) != len(cmdList)+1 {
			//	fmt.Println("analyzeWarningRule:", "len(tagIDList) != len(cmdList)+1")
			//	return
			//}
			for _, tagIDInList := range tagIDList {
				//if tagID != tagIDInList {
				if resultIndex >= len(cmdList) {
					break
				}
				if cmdList[resultIndex].Err() != nil {
					logger.Errorf(eventComputeLogicLog, "Redis批量查询中查询条件为%+v的查询结果出现错误", cmdList[resultIndex].Args())
					return fmt.Errorf("Redis批量查询中查询条件为%+v的查询结果出现错误", cmdList[resultIndex].Args())
				} else {
					if _, ok := fieldsMap[tagIDInList]; ok {
						continue
					}
					//如果公式中的该参数为输入值类型则不用查Redis，直接套用
					if _, ok := inputMap[tagIDInList]; ok {
						//logicMap[tagIDInList] = inputVal
						continue
					}
					//resVal, err := tools.InterfaceTypeToRedisMethod(cmdList[resultIndex])
					//if err != nil {
					//	return
					//}
					testAbnormalVal := cmdList[resultIndex].String()
					if !tools.IsNumber(testAbnormalVal) {
						logger.Errorf(eventComputeLogicLog, "Redis批量查询中查询条件为%+v的查询结果不是合法数字(%s)", cmdList[resultIndex].Args(), testAbnormalVal)
						continue ruleloop
					}
					resVal, err := cmdList[resultIndex].Float64()
					if err != nil {
						logger.Errorf(eventComputeLogicLog, "Redis批量查询中查询条件为%+v的查询结果数值类型不为float64", cmdList[resultIndex].Args())
						return fmt.Errorf("Redis批量查询中查询条件为%+v的查询结果数值类型不为float64", cmdList[resultIndex].Args())
					}
					logicMap[tagIDInList] = resVal
					resultIndex++
				}
			}

			//=====
			nodePropMap := map[string]interface{}{}
			//找资产属性
			for _, nodeProp := range nodePropList {
				if nodePropInCache, ok := nodeInfo[nodeProp]; ok {
					nodePropMap[nodeProp] = nodePropInCache
				}
			}
			applyLogicMap := map[string]interface{}{
				"var":  logicMap,
				"node": nodePropMap,
			}
			logicByte, err := json.Marshal(logic)
			if err != nil {
				logger.Errorf(eventComputeLogicLog, "动态属性报警规则计算公式序列化失败:%s", err.Error())
				return fmt.Errorf("动态属性报警规则计算公式序列化失败:%s", err.Error())
			}
			logicInterfaceMap := strings.NewReader(string(logicByte))
			dataByte, err := json.Marshal(applyLogicMap)
			if err != nil {
				logger.Errorf(eventComputeLogicLog, "动态属性报警规则实时数据序列化失败:%s", err.Error())
				return fmt.Errorf("动态属性报警规则实时数据序列化失败:%s", err.Error())
			}
			applyDataMap := strings.NewReader(string(dataByte))
			var result bytes.Buffer
			var computeResult bool
			logger.Debugf(eventComputeLogicLog, "资产(%s)的报警数值Map为:%+v", nodeID, logicMap)
			err = jsonlogic.Apply(logicInterfaceMap, applyDataMap, &result)
			if err != nil {
				logger.Errorf(eventComputeLogicLog, "动态属性报警规则计算公式计算失败:%s", err.Error())
				continue
			}
			decoder := json.NewDecoder(&result)
			err = decoder.Decode(&computeResult)
			if err != nil {
				logger.Errorf(eventComputeLogicLog, "动态属性计算公式解码计算结果失败:%s", err.Error())
				continue
			}
			if computeResult {
				fields := make([]map[string]interface{}, 0)
				for k, v := range logicMap {
					fieldsMap[k] = v
					tagCache, err := clogic.TagLogic.FindLocalCache(modelID, nodeID, uid, k)
					if err != nil {
						logger.Errorf(eventComputeLogicLog, "获取资产(%s)的数据点(%s)缓存失败:%s", nodeID, k, err.Error())
						continue
					}

					fields = append(fields, map[string]interface{}{
						"id":    k,
						"name":  tagCache.Name,
						"value": v,
					})
				}
				departmentStringIDList := make([]string, 0)
				//var departmentObjectList primitive.A
				if departmentIDList, ok := nodeInfo["department"].([]interface{}); ok {
					departmentStringIDList = tools.InterfaceListToStringList(departmentIDList)
				} else {
					logger.Warnf(eventComputeLogicLog, "资产(%s)的部门字段不存在或类型错误", nodeID)
				}

				deptInfoList := make([]map[string]interface{}, 0)
				if len(departmentStringIDList) != 0 {
					deptInfoList, err = clogic.DeptLogic.FindLocalCacheList(departmentStringIDList)
					if err != nil {
						return fmt.Errorf("获取当前资产(%s)所属部门失败:%s", nodeID, err.Error())
					}
				}
				//生成计算对象并发送
				sendMap := bson.M{
					"time": tools.GetLocalTimeNow(time.Now()).Format("2006-01-02 15:04:05"),
					//"status": "未处理",
					"departmentName": tools.FormatKeyInfoListMap(deptInfoList, "name"),
					"modelName":      tools.FormatKeyInfo(modelInfo, "name"),
					"nodeName":       tools.FormatKeyInfo(nodeInfo, "name"),
					"nodeUid":        tools.FormatKeyInfo(nodeInfo, "uid"),
					"tagInfo":        tools.FormatDataInfoList(fields),
					"fields":         fieldsMap,
				}
				//b, err := json.Marshal(sendMap)
				//if err != nil {
				//	logger.Errorf(eventComputeLogicLog, "要发送到事件处理器的数据消息序列化失败:%s", err.Error())
				//	return fmt.Errorf("要发送到事件处理器的数据消息序列化失败:%s", err.Error())
				//}
				//logger.Debugf(eventComputeLogicLog, "发送的数据消息为:%s", string(b))
				//imqtt.SendMsg(emqttConn, "event/"+eventID.Hex(), string(b))
				b, err := json.Marshal(sendMap)
				if err != nil {
					continue
				}
				err = imqtt.Send(fmt.Sprintf("event/%s", eventID), b)
				if err != nil {
					logger.Warnf(eventComputeLogicLog, "发送事件(%s)错误:%s", eventID, err.Error())
				} else {
					logger.Debugf(eventComputeLogicLog, "发送事件成功:%s,数据为:%+v", eventID, sendMap)
				}
			}
		}
	}

	//logger.Debugf(eventComputeLogicLog, "计算事件触发器执行结束")
	return nil
}
