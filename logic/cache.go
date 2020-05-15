package logic

import (
	"fmt"
	"sync"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/json-iterator/go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	"github.com/air-iot/service/db/redis"
	"github.com/air-iot/service/model"
	"github.com/air-iot/service/mq/mqtt"
	"github.com/air-iot/service/tools"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

const (
	ConfigCacheChannel      = "config_cache_channel"
	ConfigCache             = "config_cache"
	ConfigEventCache        = "config_event_cache"
	ConfigEventHandlerCache = "config_event_handler_cache"
	ConfigDeptCache         = "config_dept_cache"
	ConfigUserCache         = "config_user_cache"
	ConfigSettingCache      = "config_setting_cache"
)

type CacheM struct {
	Model []map[string]interface{} `json:"model"`
	Node  []map[string]interface{} `json:"node"`
}

type Cache struct {
	Model []model.Model `json:"model"`
	Node  []model.Node  `json:"node"`
}

type EventCache struct {
	Event []model.Event `json:"event"`
}

type EventHandlerCache struct {
	EventHandler []model.EventHandler `json:"eventHandler"`
}

type DeptCacheM struct {
	Dept []map[string]interface{} `json:"dept"`
}

type UserCacheM struct {
	User []map[string]interface{} `json:"user"`
}

type SettingCache struct {
	Setting model.Setting `json:"setting"`
}

func Init() {
	if !viper.GetBool("mqtt.enable") || !viper.GetBool("redis.enable") || !viper.GetBool("cache.enable") {
		return
	}
	NodeLogic.nodeCache = &sync.Map{}
	NodeLogic.nodeMapCache = &sync.Map{}
	NodeLogic.nodeCacheWithModelID = &sync.Map{}
	NodeLogic.nodeUidMapCache = &sync.Map{}
	ModelLogic.modelCache = &sync.Map{}
	ModelLogic.modelNapCache = &sync.Map{}
	TagLogic.tagCache = &sync.Map{}
	EventLogic.eventCache = &sync.Map{}
	EventLogic.eventCacheWithEventID = &sync.Map{}
	EventHandlerLogic.eventHandlerCache = &sync.Map{}
	SettingLogic.settingCache = &sync.Map{}

	cache()
	cacheEvent()
	cacheEventHandler()
	cacheDept()
	cacheUser()
	cacheSetting()
	if token := mqtt.Client.Subscribe(ConfigCacheChannel, 0, func(client MQTT.Client, message MQTT.Message) {
		logrus.Debugf("更新缓存:%s", string(message.Payload()))
		switch string(message.Payload()) {
		case ConfigCache:
			go func() {
				err := cache()
				if err != nil {
					logrus.Errorf("更新模型及节点缓存错误:%s", err.Error())
				}
			}()
		case ConfigEventCache:
			go func() {
				err := cacheEvent()
				if err != nil {
					logrus.Errorf("更新事件缓存错误:%s", err.Error())
				}
			}()
		case ConfigEventHandlerCache:
			go func() {
				err := cacheEventHandler()
				if err != nil {
					logrus.Errorf("更新事件handler缓存错误:%s", err.Error())
				}
			}()
		case ConfigDeptCache:
			go func() {
				err := cacheDept()
				if err != nil {
					logrus.Errorf("更新部门缓存错误:%s", err.Error())
				}
			}()
		case ConfigUserCache:
			go func() {
				err := cacheUser()
				if err != nil {
					logrus.Errorf("更新用户缓存错误:%s", err.Error())
				}
			}()
		case ConfigSettingCache:
			go func() {
				err := cacheSetting()
				if err != nil {
					logrus.Errorf("更新系统配置缓存错误:%s", err.Error())
				}
			}()
		}
	}); token.Wait() && token.Error() != nil {
		logrus.Errorf("更新缓存接收消息错误:%s", token.Error().Error())
	}

}

func cache() error {
	cmd := redis.Client.Get(ConfigCache)
	if cmd.Err() == nil {
		result := new(Cache)
		if err := json.Unmarshal([]byte(cmd.Val()), &result); err != nil {
			return fmt.Errorf("解析结构错误:%s", err.Error())
		}
		resultMap := new(CacheM)
		if err := json.Unmarshal([]byte(cmd.Val()), &resultMap); err != nil {
			return fmt.Errorf("解析Map错误:%s", err.Error())
		}

		for _, n := range result.Model {
			ModelLogic.modelCache.Store(n.ID, n)
		}

		for _, n := range resultMap.Model {
			if id, ok := n["id"]; ok {
				ModelLogic.modelNapCache.Store(id, n)
			}
		}

		nodeCacheModelIDMapRaw := &map[string][]model.Node{}
		for _, n := range result.Node {
			if n.Model != "" {
				tools.MergeNodeDataMap(n.Model, n, nodeCacheModelIDMapRaw)
			}
			NodeLogic.nodeCache.Store(n.ID, n)
			NodeLogic.nodeUidMapCache.Store(n.Uid, n)
		}
		for k, v := range *nodeCacheModelIDMapRaw {
			NodeLogic.nodeCacheWithModelID.Store(k, v)
		}

		for _, n := range resultMap.Node {
			if id, ok := n["id"]; ok {
				NodeLogic.nodeMapCache.Store(id, n)
			}
		}
		//TagLogic.Mutex.Lock()
		TagLogic.tagCache = &sync.Map{}
		//TagLogic.Mutex.Unlock()
	}
	return nil
}

func cacheEvent() error {
	cmd := redis.Client.Get(ConfigEventCache)
	if cmd.Err() == nil {
		result := new(EventCache)
		if err := json.Unmarshal([]byte(cmd.Val()), &result); err != nil {
			return fmt.Errorf("解析错误:%s", err.Error())
		}

		eventCacheMapRaw := &map[string][]model.Event{}
		for _, n := range result.Event {
			EventLogic.eventCacheWithEventID.Store(n.ID, n)

			if modelID, ok := n.Settings["model"].(string); ok {
				if n.Type != "" && modelID != "" {
					tools.MergeEventDataMap(modelID+"|"+n.Type, n, eventCacheMapRaw)
				}
			}
		}
		for k, v := range *eventCacheMapRaw {
			EventLogic.eventCache.Store(k, v)
		}
	}
	return nil
}

func cacheEventHandler() error {
	cmd := redis.Client.Get(ConfigEventHandlerCache)
	if cmd.Err() == nil {
		result := new(EventHandlerCache)
		if err := json.Unmarshal([]byte(cmd.Val()), &result); err != nil {
			return fmt.Errorf("解析错误:%s", err.Error())
		}

		eventHandlerCacheMapRaw := &map[string][]model.EventHandler{}
		for _, n := range result.EventHandler {
			if n.Event != "" {
				tools.MergeEventHandlerDataMap(n.Event, n, eventHandlerCacheMapRaw)
			}
			if n.Type != "" {
				tools.MergeEventHandlerDataMap(n.Type, n, eventHandlerCacheMapRaw)
			}
		}
		for k, v := range *eventHandlerCacheMapRaw {
			EventHandlerLogic.eventHandlerCache.Store(k, v)
		}
	}
	return nil
}

func cacheDept() error {
	cmd := redis.Client.Get(ConfigDeptCache)
	if cmd.Err() == nil {
		result := new(DeptCacheM)
		if err := json.Unmarshal([]byte(cmd.Val()), &result); err != nil {
			return fmt.Errorf("解析错误:%s", err.Error())
		}

		for _, n := range result.Dept {
			if id, ok := n["id"]; ok {
				DeptLogic.deptMapCache.Store(id, n)
			}
		}
	}
	return nil
}

func cacheUser() error {
	cmd := redis.Client.Get(ConfigUserCache)
	if cmd.Err() == nil {
		resultMap := new(UserCacheM)
		if err := json.Unmarshal([]byte(cmd.Val()), &resultMap); err != nil {
			return fmt.Errorf("解析Map错误:%s", err.Error())
		}

		for _, n := range resultMap.User {
			if id, ok := n["id"]; ok {
				UserLogic.userMapCache.Store(id, n)
			}
		}

	}
	return nil
}

func cacheSetting() error {
	cmd := redis.Client.Get(ConfigSettingCache)
	if cmd.Err() == nil {
		result := new(SettingCache)
		if err := json.Unmarshal([]byte(cmd.Val()), &result); err != nil {
			return fmt.Errorf("解析错误:%s", err.Error())
		}

		SettingLogic.settingCache.Store("setting", *result)
	}
	return nil
}
