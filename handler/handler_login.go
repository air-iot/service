package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/air-iot/service/model"
	"time"

	idb "github.com/air-iot/service/db/mongo"
	"github.com/air-iot/service/logger"
	clogic "github.com/air-iot/service/logic"
	imqtt "github.com/air-iot/service/mq/mqtt"
	"github.com/air-iot/service/restful-api"
	"github.com/air-iot/service/tools"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var eventLoginLog = map[string]interface{}{"name": "用户登录事件触发"}

func TriggerLogin(data map[string]interface{}) error {
	//logger.Debugf(eventLoginLog, "开始执行计算事件触发器")
	//logger.Debugf(eventLoginLog, "传入参数为:%+v", data)

	//logger.Debugf(eventLoginLog, "开始获取当前模型的用户登录事件")
	//获取当前用户登录事件=============================================
	eventInfoList, err := clogic.EventLogic.FindLocalCacheByType(string(Login))
	if err != nil {
		logger.Debugf(eventLoginLog, fmt.Sprintf("获取用户登录事件失败:%s", err.Error()))
		return fmt.Errorf("获取用户登录事件失败:%s", err.Error())
	}

	//
	//modelInfo, err := clogic.ModelLogic.FindLocalMapCache(modelID)
	//if err != nil {
	//	logger.Errorf(eventLoginLog, fmt.Sprintf("获取当前模型(%s)详情失败:%s", modelID, err.Error()))
	//	return fmt.Errorf("获取当前模型(%s)详情失败:%s", modelID, err.Error())
	//}
	userID := ""
	if v, ok := data["user"].(string); ok {
		userID = v
	}

	departmentListInSettings := make([]primitive.ObjectID, 0)
	roleListInSettings := make([]primitive.ObjectID, 0)
	departmentStringListInSettings := make([]string, 0)
	roleStringListInSettings := make([]string, 0)
	//logger.Debugf(eventLoginLog, "开始遍历事件列表")
	for _, eventInfo := range *eventInfoList {
		logger.Debugf(eventLoginLog, "事件信息为:%+v", eventInfo)
		if eventInfo.Handlers == nil || len(eventInfo.Handlers) == 0 {
			logger.Warnln(eventLoginLog, "handlers字段数组长度为0")
			continue
		}
		logger.Debugf(eventLoginLog, "开始分析事件")
		eventID := eventInfo.ID
		settings := eventInfo.Settings

		//判断是否已经失效
		if invalid, ok := settings["invalid"].(bool); ok {
			if invalid {
				logger.Warnln(eventLog, "事件(%s)已经失效", eventID)
				continue
			}
		}

		//判断禁用
		if disable, ok := settings["disable"].(bool); ok {
			if disable {
				logger.Warnln(eventLoginLog, "事件(%s)已经被禁用", eventID)
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
							formatStartTime, err := tools.ConvertStringToTime("2006-01-02 15:04:05", startTime, time.Local)
							if err != nil {
								//logger.Errorf(logFieldsMap, "时间范围字段值格式错误:%s", err.Error())
								formatStartTime, err = tools.ConvertStringToTime("2006-01-02T15:04:05+08:00", startTime, time.Local)
								if err != nil {
									logger.Errorf(eventLoginLog, "时间范围字段值格式错误:%s", err.Error())
									continue
								}
								//return restfulapi.NewHTTPError(http.StatusBadRequest, "startTime", fmt.Sprintf("时间范围字段格式错误:%s", err.Error()))
							}
							if tools.GetLocalTimeNow(time.Now()).Unix() < formatStartTime.Unix() {
								logger.Debugf(eventLoginLog, "事件(%s)的定时任务开始时间未到，不执行", eventID)
								continue
							}
						}

						if endTime, ok := settings["endTime"].(string); ok {
							formatEndTime, err := tools.ConvertStringToTime("2006-01-02 15:04:05", endTime, time.Local)
							if err != nil {
								//logger.Errorf(logFieldsMap, "时间范围字段值格式错误:%s", err.Error())
								formatEndTime, err = tools.ConvertStringToTime("2006-01-02T15:04:05+08:00", endTime, time.Local)
								if err != nil {
									logger.Errorf(eventLoginLog, "时间范围字段值格式错误:%s", err.Error())
									continue
								}
								//return restfulapi.NewHTTPError(http.StatusBadRequest, "startTime", fmt.Sprintf("时间范围字段格式错误:%s", err.Error()))
							}
							if tools.GetLocalTimeNow(time.Now()).Unix() >= formatEndTime.Unix() {
								logger.Debugf(eventLoginLog, "事件(%s)的定时任务结束时间已到，不执行", eventID)
								//修改事件为失效
								updateMap := bson.M{"settings.invalid": true}
								_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
								if err != nil {
									logger.Errorf(eventLoginLog, "失效事件(%s)失败:%s", eventID, err.Error())
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
			err = tools.FormatObjectIDListMap(&settings, "department", "id")
			if err != nil {
				logger.Warnln(eventLoginLog, "事件配置的部门对象中id字段不存在或类型错误")
				continue
			}
			err = tools.FormatObjectIDListMap(&settings, "role", "id")
			if err != nil {
				logger.Warnln(eventLoginLog, "事件配置的角色对象中id字段不存在或类型错误")
				continue
			}
			departmentListInSettings, ok = settings["department"].([]primitive.ObjectID)
			if !ok {
				departmentListInSettings = make([]primitive.ObjectID, 0)
			}
			roleListInSettings, ok = settings["role"].([]primitive.ObjectID)
			if !ok {
				roleListInSettings = make([]primitive.ObjectID, 0)
			}
			if len(departmentListInSettings) != 0 && len(roleListInSettings) != 0 {
				departmentStringListInSettings, err = tools.ObjectIdListToStringList(departmentListInSettings)
				if err != nil {
					logger.Warnln(eventLoginLog, "部门ObjectID转ID失败:%s", err.Error())
					continue
				}
				roleStringListInSettings, err = tools.ObjectIdListToStringList(roleListInSettings)
				if err != nil {
					logger.Warnln(eventLoginLog, "角色ObjectID转ID失败:%s", err.Error())
					continue
				}
				userInfoListByDept, err := clogic.UserLogic.FindLocalDeptUserCacheList(departmentStringListInSettings)
				if err != nil {
					logger.Warnln(eventLoginLog, "部门ID获取用户缓存失败:%s", err.Error())
					continue
				}
				userInfoListByRole, err := clogic.UserLogic.FindLocalRoleUserCacheList(roleStringListInSettings)
				if err != nil {
					logger.Warnln(eventLoginLog, "角色ID获取用户缓存失败:%s", err.Error())
					continue
				}

				userIDListRaw := make([]string, 0)
				for _, user := range *userInfoListByDept {
					userIDListRaw = tools.AddNonRepByLoop(userIDListRaw, user.ID)
				}
				for _, user := range *userInfoListByRole {
					for _, userID := range userIDListRaw {
						if userID == user.ID {
							userIDList = tools.AddNonRepByLoop(userIDList, user.ID)
						}
					}
				}

			} else if len(departmentListInSettings) != 0 && len(roleListInSettings) == 0 {
				departmentStringListInSettings, err = tools.ObjectIdListToStringList(departmentListInSettings)
				if err != nil {
					logger.Warnln(eventLoginLog, "部门ObjectID转ID失败:%s", err.Error())
					continue
				}
				userInfoListByDept, err := clogic.UserLogic.FindLocalDeptUserCacheList(departmentStringListInSettings)
				if err != nil {
					logger.Warnln(eventLoginLog, "部门ID获取用户缓存失败:%s", err.Error())
					continue
				}

				for _, user := range *userInfoListByDept {
					userIDList = tools.AddNonRepByLoop(userIDList, user.ID)
				}

			} else if len(departmentListInSettings) == 0 && len(roleListInSettings) != 0 {
				roleStringListInSettings, err = tools.ObjectIdListToStringList(roleListInSettings)
				if err != nil {
					logger.Warnln(eventLoginLog, "角色ObjectID转ID失败:%s", err.Error())
					continue
				}
				userInfoListByRole, err := clogic.UserLogic.FindLocalRoleUserCacheList(roleStringListInSettings)
				if err != nil {
					logger.Warnln(eventLoginLog, "角色ID获取用户缓存失败:%s", err.Error())
					continue
				}

				for _, user := range *userInfoListByRole {
					userIDList = tools.AddNonRepByLoop(userIDList, user.ID)
				}
			} else {
				continue
			}
		} else {
			err = tools.FormatObjectIDListMap(&settings, "users", "id")
			if err != nil {
				logger.Errorf(eventLoginLog, "数据信息中用户字段中id字段不存在或类型错误:%s", err.Error())
				continue
			}
			userObjectIDList, ok := settings["users"].([]primitive.ObjectID)
			if !ok {
				logger.Errorf(eventLoginLog, "数据信息中用户ID数组字段不存在或类型错误")
				continue
			}
			userIDList, err = tools.ObjectIdListToStringList(userObjectIDList)
			if err != nil {
				logger.Errorf(eventLoginLog, fmt.Sprintf("当前用户登录触发器(%s)的用户ID转字符串数组失败:%s", eventID, err.Error()))
				continue
			}
		}

		if !allUser {
			isValid := false
			for _, id := range userIDList {
				if userID == id {
					departmentStringIDList := make([]string, 0)
					//var departmentObjectList primitive.A
					if departmentIDList, ok := data["department"].([]interface{}); ok {
						departmentStringIDList = tools.InterfaceListToStringList(departmentIDList)
					} else {
						logger.Warnf(eventLoginLog, "用户(%s)所属部门字段不存在或类型错误", userID)
					}

					deptInfoList := make([]map[string]interface{}, 0)
					if len(departmentStringIDList) != 0 {
						deptInfoList, err = clogic.DeptLogic.FindLocalCacheList(departmentStringIDList)
						if err != nil {
							logger.Warnf(eventLoginLog, "获取当前用户(%s)所属部门失败:%s", userID, err.Error())
						}
						data["departmentName"] = tools.FormatKeyInfoListMap(deptInfoList, "name")
					}

					userInfo := &model.User{}
					userInfo, err := clogic.UserLogic.FindLocalCache(userID)
					if err != nil {
						logger.Warnf(eventLoginLog, "获取用户(%s)缓存失败:%s", userID, err.Error())
						userInfo = &model.User{}
					}

					roleIDs := userInfo.Roles
					roleInfoList := make([]map[string]interface{}, 0)
					if len(roleIDs) != 0 {
						roleInfoList, err = clogic.RoleLogic.FindLocalMapCacheList(roleIDs)
						if err != nil {
							logger.Warnf(eventLoginLog, "获取当前用户(%s)拥有的角色失败:%s", userID, err.Error())
						}
						data["roleName"] = tools.FormatKeyInfoListMap(roleInfoList, "name")
					}

					isValid = true
					break
				}
			}

			if !isValid {
				continue
			}
		}

		sendMap := data
		b, err := json.Marshal(sendMap)
		if err != nil {
			continue
		}
		err = imqtt.Send(fmt.Sprintf("event/%s", eventID), b)
		if err != nil {
			logger.Warnf(eventLoginLog, "发送事件(%s)错误:%s", eventID, err.Error())
		} else {
			logger.Debugf(eventLoginLog, "发送事件成功:%s,数据为:%+v", eventID, sendMap)
		}

		hasExecute = true

		//对只能执行一次的事件进行失效
		if validTime == "timeLimit" {
			if rangeDefine == "once" && hasExecute {
				logger.Warnln(eventLoginLog, "事件(%s)为只执行一次的事件", eventID)
				//修改事件为失效
				updateMap := bson.M{"settings.invalid": true}
				_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
				if err != nil {
					logger.Errorf(eventLoginLog, "失效事件(%s)失败:%s", eventID, err.Error())
					continue
				}
			}
		}

	}

	//logger.Debugf(eventLoginLog, "计算事件触发器执行结束")
	return nil
}
