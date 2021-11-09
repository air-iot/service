package flow

import (
	"context"
	"fmt"
	"github.com/air-iot/service/init/cache/flow"
	"github.com/air-iot/service/logger"
	"github.com/air-iot/service/util/flowx"
	"github.com/camunda-cloud/zeebe/clients/go/pkg/zbc"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/air-iot/service/api"
	"github.com/air-iot/service/gin/ginx"
	"github.com/air-iot/service/init/cache/department"
	"github.com/air-iot/service/init/cache/entity"
	"github.com/air-iot/service/init/cache/role"
	"github.com/air-iot/service/init/cache/user"
	"github.com/air-iot/service/init/mq"
	"github.com/air-iot/service/init/redisdb"
	"github.com/air-iot/service/util/formatx"
	"github.com/air-iot/service/util/timex"
)

var flowLoginLog = map[string]interface{}{"name": "用户登录流程触发"}

func TriggerLoginFlow(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, mq mq.MQ, apiClient api.Client, zbClient zbc.Client, projectName string, data map[string]interface{}) error {
	////logger.Debugf(eventLoginLog, "开始执行计算流程触发器")
	////logger.Debugf(eventLoginLog, "传入参数为:%+v", data)

	////logger.Debugf(eventLoginLog, "开始获取当前模型的用户登录流程")
	//获取当前用户登录流程=============================================
	headerMap := map[string]string{ginx.XRequestProject: projectName}
	flowInfoList := new([]entity.Flow)
	err := flow.GetByType(ctx, redisClient, mongoClient, projectName, string(Login), flowInfoList)
	if err != nil {
		//logger.Debugf(flowLoginLog, fmt.Sprintf("获取用户登录流程失败:%s", err.Error()))
		return fmt.Errorf("获取用户登录流程失败:%s", err.Error())
	}

	//
	//modelInfo, err := clogic.ModelLogic.FindLocalMapCache(modelID)
	//if err != nil {
	//	//logger.Errorf(flowLoginLog, fmt.Sprintf("获取当前模型(%s)详情失败:%s", modelID, err.Error()))
	//	return fmt.Errorf("获取当前模型(%s)详情失败:%s", modelID, err.Error())
	//}
	userID := ""
	if v, ok := data["user"].(string); ok {
		userID = v
	}

	departmentListInSettings := make([]string, 0)
	roleListInSettings := make([]string, 0)
	////logger.Debugf(flowLoginLog, "开始遍历流程列表")
	for _, flowInfo := range *flowInfoList {
		//logger.Debugf(eventLoginLog, "流程信息为:%+v", eventInfo)
		//logger.Debugf(eventLoginLog, "开始分析流程")
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
				var r = make(map[string]interface{})
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

		userIDList := make([]string, 0)
		sendTo := ""
		if v, ok := settings["sendTo"].(string); ok {
			sendTo = v
		}
		allUser := false
		if sendTo == "all" {
			allUser = true
		} else if sendTo == "deptRole" {
			ok := false
			//没有指定特定用户时，结合部门与角色进行判断
			err = formatx.FormatObjectToIDListMap(&settings, "department", "id")
			if err != nil {
				//logger.Warnln(flowLoginLog, "流程配置的部门对象中id字段不存在或类型错误")
				continue
			}
			err = formatx.FormatObjectToIDListMap(&settings, "role", "id")
			if err != nil {
				//logger.Warnln(flowLoginLog, "流程配置的角色对象中id字段不存在或类型错误")
				continue
			}
			departmentListInSettings, ok = settings["department"].([]string)
			if !ok {
				departmentListInSettings = make([]string, 0)
			}
			roleListInSettings, ok = settings["role"].([]string)
			if !ok {
				roleListInSettings = make([]string, 0)
			}
			if len(departmentListInSettings) != 0 && len(roleListInSettings) != 0 {
				userInfoListByDept := new([]entity.User)
				err := user.GetByDept(ctx, redisClient, mongoClient, projectName, departmentListInSettings, userInfoListByDept)
				if err != nil {
					//logger.Warnln(flowLoginLog, "部门ID获取用户缓存失败:%s", err.Error())
					continue
				}
				userInfoListByRole := new([]entity.User)
				err = user.GetByRole(ctx, redisClient, mongoClient, projectName, roleListInSettings, userInfoListByRole)
				if err != nil {
					//logger.Warnln(flowLoginLog, "角色ID获取用户缓存失败:%s", err.Error())
					continue
				}

				userIDListRaw := make([]string, 0)
				for _, user := range *userInfoListByDept {
					userIDListRaw = formatx.AddNonRepByLoop(userIDListRaw, user.ID)
				}
				for _, user := range *userInfoListByRole {
					for _, userID := range userIDListRaw {
						if userID == user.ID {
							userIDList = formatx.AddNonRepByLoop(userIDList, user.ID)
						}
					}
				}

			} else if len(departmentListInSettings) != 0 && len(roleListInSettings) == 0 {
				userInfoListByDept := new([]entity.User)
				err := user.GetByDept(ctx, redisClient, mongoClient, projectName, departmentListInSettings, userInfoListByDept)
				if err != nil {
					//logger.Warnln(flowLoginLog, "部门ID获取用户缓存失败:%s", err.Error())
					continue
				}

				for _, user := range *userInfoListByDept {
					userIDList = formatx.AddNonRepByLoop(userIDList, user.ID)
				}

			} else if len(departmentListInSettings) == 0 && len(roleListInSettings) != 0 {
				userInfoListByRole := new([]entity.User)
				err = user.GetByRole(ctx, redisClient, mongoClient, projectName, roleListInSettings, userInfoListByRole)
				if err != nil {
					//logger.Warnln(flowLoginLog, "角色ID获取用户缓存失败:%s", err.Error())
					continue
				}

				for _, user := range *userInfoListByRole {
					userIDList = formatx.AddNonRepByLoop(userIDList, user.ID)
				}
			} else {
				continue
			}
		} else {
			err = formatx.FormatObjectToIDListMap(&settings, "users", "id")
			if err != nil {
				//logger.Errorf(flowLoginLog, "数据信息中用户字段中id字段不存在或类型错误:%s", err.Error())
				continue
			}
			var ok bool
			userIDList, ok = settings["users"].([]string)
			if !ok {
				//logger.Errorf(flowLoginLog, "数据信息中用户ID数组字段不存在或类型错误")
				continue
			}
		}

		if !allUser {
			isValid := false
			for _, id := range userIDList {
				if userID == id {
					isValid = true
					break
				}
			}

			if !isValid {
				continue
			}
		}

		departmentStringIDList := make([]string, 0)
		//var departmentObjectList primitive.A
		if departmentIDList, ok := data["department"].([]interface{}); ok {
			departmentStringIDList = formatx.InterfaceListToStringList(departmentIDList)
		} else {
			//logger.Warnf(flowLoginLog, "用户(%s)所属部门字段不存在或类型错误", userID)
		}

		deptInfoList := make([]map[string]interface{}, 0)
		if len(departmentStringIDList) != 0 {
			err := department.GetByList(ctx, redisClient, mongoClient, projectName, departmentStringIDList, &deptInfoList)
			if err != nil {
				//logger.Warnln(flowLoginLog, "部门ID获取用户缓存失败:%s", err.Error())
			}
			data["departmentName"] = formatx.FormatKeyInfoListMap(deptInfoList, "name")
		}

		userInfo := &entity.User{}
		err = user.Get(ctx, redisClient, mongoClient, projectName, userID, userInfo)
		if err != nil {
			//logger.Warnf(flowLoginLog, "获取用户(%s)缓存失败:%s", userID, err.Error())
			userInfo = &entity.User{}
		}

		roleIDs := userInfo.Roles
		roleInfoList := make([]map[string]interface{}, 0)
		if len(roleIDs) != 0 {
			err := role.GetByList(ctx, redisClient, mongoClient, projectName, roleIDs, &roleInfoList)
			if err != nil {
				//logger.Warnf(flowLoginLog, "获取当前用户(%s)拥有的角色失败:%s", userID, err.Error())
			}
			data["roleName"] = formatx.FormatKeyInfoListMap(roleInfoList, "name")
		}

		deptMap := bson.M{}
		for _, id := range departmentStringIDList {
			deptMap[id] = bson.M{"id": id, "_tableName": "dept"}
		}
		roleMap := bson.M{}
		for _, id := range roleIDs {
			roleMap[id] = bson.M{"id": id, "_tableName": "role"}
		}
		data["#$department"] = deptMap
		data["#$role"] = roleMap
		data["#$user"] = bson.M{"id": userID, "_tableName": "user"}
		//if loginTimeRaw, ok := data["time"].(string); ok {
		//	loginTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(loginTimeRaw), loginTimeRaw, time.Local)
		//	if err != nil {
		//		continue
		//	}
		//	data["time"] = loginTime.UnixNano() / 1e6
		//}

		err = flowx.StartFlow(mongoClient,zbClient, flowInfo.FlowXml, projectName,string(LoginTrigger),flowID, data,settings)
		if err != nil {
			return fmt.Errorf("流程推进到下一阶段失败:%s", err.Error())
		}

		hasExecute = true

		//对只能执行一次的流程进行失效
		if flowInfo.ValidTime == "timeLimit" {
			if flowInfo.Range == "once" && hasExecute {
				//logger.Warnln(eventLoginLog, "流程(%s)为只执行一次的流程", eventID)
				//修改流程为失效
				updateMap := bson.M{"invalid": true}
				//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
				var r = make(map[string]interface{})
				err := apiClient.UpdateFlowById(headerMap, flowID, updateMap, &r)
				if err != nil {
					//logger.Errorf(flowLoginLog, "失效流程(%s)失败:%s", flowID, err.Error())
					continue
				}
			}
		}

	}

	////logger.Debugf(eventLoginLog, "计算流程触发器执行结束")
	return nil
}
