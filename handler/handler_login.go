package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/air-iot/service/api"
	"github.com/air-iot/service/gin/ginx"
	"github.com/air-iot/service/init/cache/department"
	"github.com/air-iot/service/init/cache/entity"
	"github.com/air-iot/service/init/cache/event"
	"github.com/air-iot/service/init/cache/role"
	"github.com/air-iot/service/init/cache/user"
	"github.com/air-iot/service/init/mq"
	"github.com/air-iot/service/util/formatx"
	"github.com/air-iot/service/util/timex"
	"github.com/go-redis/redis/v8"
	"go.mongodb.org/mongo-driver/mongo"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

var eventLoginLog = map[string]interface{}{"name": "用户登录事件触发"}

func TriggerLogin(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client,mq mq.MQ, apiClient api.Client,projectName string,data map[string]interface{}) error {
	////logger.Debugf(eventLoginLog, "开始执行计算事件触发器")
	////logger.Debugf(eventLoginLog, "传入参数为:%+v", data)

	////logger.Debugf(eventLoginLog, "开始获取当前模型的用户登录事件")
	//获取当前用户登录事件=============================================
	headerMap := map[string]string{ginx.XRequestProject:projectName}
	eventInfoList := new([]entity.Event)
	err := event.GetByType(ctx,redisClient,mongoClient,projectName,string(Login),eventInfoList)
	if err != nil {
		//logger.Debugf(eventLoginLog, fmt.Sprintf("获取用户登录事件失败:%s", err.Error()))
		return fmt.Errorf("获取用户登录事件失败:%s", err.Error())
	}

	//
	//modelInfo, err := clogic.ModelLogic.FindLocalMapCache(modelID)
	//if err != nil {
	//	//logger.Errorf(eventLoginLog, fmt.Sprintf("获取当前模型(%s)详情失败:%s", modelID, err.Error()))
	//	return fmt.Errorf("获取当前模型(%s)详情失败:%s", modelID, err.Error())
	//}
	userID := ""
	if v, ok := data["user"].(string); ok {
		userID = v
	}

	departmentListInSettings := make([]string, 0)
	roleListInSettings := make([]string, 0)
	////logger.Debugf(eventLoginLog, "开始遍历事件列表")
	for _, eventInfo := range *eventInfoList {
		//logger.Debugf(eventLoginLog, "事件信息为:%+v", eventInfo)
		if eventInfo.Handlers == nil || len(eventInfo.Handlers) == 0 {
			//logger.Warnln(eventLoginLog, "handlers字段数组长度为0")
			continue
		}
		//logger.Debugf(eventLoginLog, "开始分析事件")
		eventID := eventInfo.ID
		settings := eventInfo.Settings

		//判断是否已经失效
		if invalid, ok := settings["invalid"].(bool); ok {
			if invalid {
				//logger.Warnln(eventLog, "事件(%s)已经失效", eventID)
				continue
			}
		}

		//判断禁用
		if disable, ok := settings["disable"].(bool); ok {
			if disable {
				//logger.Warnln(eventLoginLog, "事件(%s)已经被禁用", eventID)
				continue
			}
		}

		rangeDefine := ""
		validTime, ok := settings["validTime"].(string)
		if ok {
			if validTime == "timeLimit" {
				if rangeDefine, ok = settings["range"].(string); ok {
					if rangeDefine != "once" {
						//判断有效期
						if startTime, ok := settings["startTime"].(string); ok {
							formatStartTime, err := timex.ConvertStringToTime("2006-01-02 15:04:05", startTime, time.Local)
							if err != nil {
								////logger.Errorf(logFieldsMap, "时间范围字段值格式错误:%s", err.Error())
								formatStartTime, err = timex.ConvertStringToTime("2006-01-02T15:04:05+08:00", startTime, time.Local)
								if err != nil {
									//logger.Errorf(eventLoginLog, "时间范围字段值格式错误:%s", err.Error())
									continue
								}
								//return restfulapi.NewHTTPError(http.StatusBadRequest, "startTime", fmt.Sprintf("时间范围字段格式错误:%s", err.Error()))
							}
							if timex.GetLocalTimeNow(time.Now()).Unix() < formatStartTime.Unix() {
								//logger.Debugf(eventLoginLog, "事件(%s)的定时任务开始时间未到，不执行", eventID)
								continue
							}
						}

						if endTime, ok := settings["endTime"].(string); ok {
							formatEndTime, err := timex.ConvertStringToTime("2006-01-02 15:04:05", endTime, time.Local)
							if err != nil {
								////logger.Errorf(logFieldsMap, "时间范围字段值格式错误:%s", err.Error())
								formatEndTime, err = timex.ConvertStringToTime("2006-01-02T15:04:05+08:00", endTime, time.Local)
								if err != nil {
									//logger.Errorf(eventLoginLog, "时间范围字段值格式错误:%s", err.Error())
									continue
								}
								//return restfulapi.NewHTTPError(http.StatusBadRequest, "startTime", fmt.Sprintf("时间范围字段格式错误:%s", err.Error()))
							}
							if timex.GetLocalTimeNow(time.Now()).Unix() >= formatEndTime.Unix() {
								//logger.Debugf(eventLoginLog, "事件(%s)的定时任务结束时间已到，不执行", eventID)
								//修改事件为失效
								updateMap := bson.M{"settings.invalid": true}
								//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
								var r = make(map[string]interface{})
								err := apiClient.UpdateEventById(headerMap,eventID, updateMap, &r)
								if err != nil {
									//logger.Errorf(eventLoginLog, "失效事件(%s)失败:%s", eventID, err.Error())
									continue
								}
								continue
							}
						}
					}
				}
			}
		}

		//判断事件是否已经触发
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
				//logger.Warnln(eventLoginLog, "事件配置的部门对象中id字段不存在或类型错误")
				continue
			}
			err = formatx.FormatObjectToIDListMap(&settings, "role", "id")
			if err != nil {
				//logger.Warnln(eventLoginLog, "事件配置的角色对象中id字段不存在或类型错误")
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
				err := user.GetByDept(ctx,redisClient,mongoClient,projectName,departmentListInSettings,userInfoListByDept)
				if err != nil {
					//logger.Warnln(eventLoginLog, "部门ID获取用户缓存失败:%s", err.Error())
					continue
				}
				userInfoListByRole := new([]entity.User)
				err = user.GetByRole(ctx,redisClient,mongoClient,projectName,roleListInSettings,userInfoListByRole)
				if err != nil {
					//logger.Warnln(eventLoginLog, "角色ID获取用户缓存失败:%s", err.Error())
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
				err := user.GetByDept(ctx,redisClient,mongoClient,projectName,departmentListInSettings,userInfoListByDept)
				if err != nil {
					//logger.Warnln(eventLoginLog, "部门ID获取用户缓存失败:%s", err.Error())
					continue
				}

				for _, user := range *userInfoListByDept {
					userIDList = formatx.AddNonRepByLoop(userIDList, user.ID)
				}

			} else if len(departmentListInSettings) == 0 && len(roleListInSettings) != 0 {
				userInfoListByRole := new([]entity.User)
				err = user.GetByRole(ctx,redisClient,mongoClient,projectName,roleListInSettings,userInfoListByRole)
				if err != nil {
					//logger.Warnln(eventLoginLog, "角色ID获取用户缓存失败:%s", err.Error())
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
				//logger.Errorf(eventLoginLog, "数据信息中用户字段中id字段不存在或类型错误:%s", err.Error())
				continue
			}
			userIDList, ok = settings["users"].([]string)
			if !ok {
				//logger.Errorf(eventLoginLog, "数据信息中用户ID数组字段不存在或类型错误")
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
			//logger.Warnf(eventLoginLog, "用户(%s)所属部门字段不存在或类型错误", userID)
		}

		deptInfoList := make([]map[string]interface{}, 0)
		if len(departmentStringIDList) != 0 {
			err := department.GetByList(ctx,redisClient,mongoClient,projectName,departmentStringIDList,&deptInfoList)
			if err != nil {
				//logger.Warnln(eventLoginLog, "部门ID获取用户缓存失败:%s", err.Error())
			}
			data["departmentName"] = formatx.FormatKeyInfoListMap(deptInfoList, "name")
		}

		userInfo := &entity.User{}
		err = user.Get(ctx,redisClient,mongoClient,projectName,userID,userInfo)
		if err != nil {
			//logger.Warnf(eventLoginLog, "获取用户(%s)缓存失败:%s", userID, err.Error())
			userInfo = &entity.User{}
		}

		roleIDs := userInfo.Roles
		roleInfoList := make([]map[string]interface{}, 0)
		if len(roleIDs) != 0 {
			err := role.GetByList(ctx,redisClient,mongoClient,projectName,roleIDs,&roleInfoList)
			if err != nil {
				//logger.Warnf(eventLoginLog, "获取当前用户(%s)拥有的角色失败:%s", userID, err.Error())
			}
			data["roleName"] = formatx.FormatKeyInfoListMap(roleInfoList, "name")
		}

		sendMap := data
		b, err := json.Marshal(sendMap)
		if err != nil {
			continue
		}
		err = mq.Publish(ctx,[]string{"event",projectName,eventID}, b)
		if err != nil {
			//logger.Warnf(eventLoginLog, "发送事件(%s)错误:%s", eventID, err.Error())
		} else {
			//logger.Debugf(eventLoginLog, "发送事件成功:%s,数据为:%+v", eventID, sendMap)
		}

		hasExecute = true

		//对只能执行一次的事件进行失效
		if validTime == "timeLimit" {
			if rangeDefine == "once" && hasExecute {
				//logger.Warnln(eventLoginLog, "事件(%s)为只执行一次的事件", eventID)
				//修改事件为失效
				updateMap := bson.M{"settings.invalid": true}
				//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
				var r = make(map[string]interface{})
				err := apiClient.UpdateEventById(headerMap,eventID, updateMap, &r)
				if err != nil {
					//logger.Errorf(eventLoginLog, "失效事件(%s)失败:%s", eventID, err.Error())
					continue
				}
			}
		}

	}

	////logger.Debugf(eventLoginLog, "计算事件触发器执行结束")
	return nil
}
