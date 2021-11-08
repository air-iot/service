package flowx

import (
	"context"
	"fmt"
	mongoOps "github.com/air-iot/service/init/mongodb"
	"github.com/camunda-cloud/zeebe/clients/go/pkg/zbc"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"time"
)

func StartFlow(mongoClient *mongo.Client, zbClient zbc.Client, flowXml, project, flowType string, data, setting interface{}) error {
	if flowXml == "" {
		return fmt.Errorf("流程配置为空")
	}
	//mv, err := mxj.NewMapJson([]byte(flowXml))
	//if err != nil {
	//	return fmt.Errorf("流程XML字符串转map失败:%s", err.Error())
	//}
	//
	//xmlValue, err := mv.Xml()
	//if err != nil {
	//	return fmt.Errorf("流程XML字符串转XML失败:%s", err.Error())
	//}
	//xmlValueFormat := `<?xml version="1.0" encoding="UTF-8"?>` + string(xmlValue)

	// deploy workflow
	ctx := context.Background()
	response, err := zbClient.NewDeployProcessCommand().AddResource([]byte(flowXml), project).Send(ctx)
	//response, err := zbClient.NewDeployWorkflowCommand().AddResourceFile("test.bpmn").Send(ctx)
	if err != nil {
		return fmt.Errorf("流程部署失败:%s", err.Error())
	}

	// create a new workflow instance
	variables := make(map[string]interface{})

	if len(response.GetProcesses()) == 0 {
		return fmt.Errorf("流程个数为0")
	}
	variables[response.GetProcesses()[0].GetBpmnProcessId()] = data
	variables["#project"] = project
	timestamp := time.Now().UnixNano() / 1e6
	flowTriggerError := FlowTriggerRecord{
		ID:                   primitive.NewObjectID().Hex(),
		Type:                 flowType,
		Variables:            variables,
		BpmnProcessID:        response.GetProcesses()[0].GetBpmnProcessId(),
		ProcessDefinitionKey: response.GetProcesses()[0].ProcessDefinitionKey,
		TimeStamp:            timestamp,
		Status:               "FAILED",
		Setting:              setting,
	}
	request, err := zbClient.NewCreateInstanceCommand().BPMNProcessId(response.GetProcesses()[0].GetBpmnProcessId()).LatestVersion().VariablesFromMap(variables)
	if err != nil {
		flowTriggerError.ErrorMessage = err.Error()
		err := writeFlowTriggerLog(ctx, mongoClient, flowTriggerError)
		if err != nil {
			return fmt.Errorf("流程触发失败日志写入失败:%s", err.Error())
		}
		return fmt.Errorf("流程实例创建失败:%s", err.Error())
	}

	instanceResponse, err := request.Send(ctx)
	if err != nil {
		flowTriggerError.ErrorMessage = err.Error()
		flowTriggerError.ProcessInstanceKey = instanceResponse.ProcessInstanceKey
		err := writeFlowTriggerLog(ctx, mongoClient, flowTriggerError)
		if err != nil {
			return fmt.Errorf("流程触发失败日志写入失败:%s", err.Error())
		}
		return fmt.Errorf("流程推进到下一阶段失败:%s", err.Error())
	}

	flowTriggerError.Status = "COMPLETED"
	flowTriggerError.ProcessInstanceKey = instanceResponse.ProcessInstanceKey
	err = writeFlowTriggerLog(ctx, mongoClient, flowTriggerError)
	if err != nil {
		return fmt.Errorf("流程触发成功日志写入失败:%s", err.Error())
	}
	return nil
}

type FlowTriggerRecord struct {
	ID                   string                 `json:"id" bson:"_id"`
	Type                 string                 `json:"type"`
	ErrorMessage         string                 `json:"errorMessage"`
	Variables            map[string]interface{} `json:"variables"`
	BpmnProcessID        string                 `json:"bpmnProcessid"`
	ProcessDefinitionKey int64                  `json:"processDefinitionKey"`
	ProcessInstanceKey   int64                  `json:"processInstanceKey"`
	TimeStamp            int64                  `json:"timestamp"`
	Status               string                 `json:"status"` //COMPLETED  FAILED
	Setting              interface{}            `json:"setting"`
}

func writeFlowTriggerLog(ctx context.Context, mongoClient *mongo.Client, data FlowTriggerRecord) error {
	_, err := mongoOps.SaveOne(ctx, mongoClient.Database("base").Collection("flow_trigger_record"), data)
	if err != nil {
		return fmt.Errorf("存储流程触发日志失败:%s", err.Error())
	}
	return nil
}
