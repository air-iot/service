package flowx

import (
	"context"
	"fmt"
	"github.com/camunda-cloud/zeebe/clients/go/pkg/zbc"
	"github.com/clbanning/mxj"
)

func StartFlow(zbClient zbc.Client, flowXml,project string, data interface{}) error {
	if flowXml == "" {
		return fmt.Errorf("流程XML字符串为空")
	}
	mv, err := mxj.NewMapJson([]byte(flowXml))
	if err != nil {
		return fmt.Errorf("流程XML字符串转map失败:%s", err.Error())
	}

	xmlValue, err := mv.Xml()
	if err != nil {
		return fmt.Errorf("流程XML字符串转XML失败:%s", err.Error())
	}
	xmlValueFormat := `<?xml version="1.0" encoding="UTF-8"?>` + string(xmlValue)

	// deploy workflow
	ctx := context.Background()
	response, err := zbClient.NewDeployProcessCommand().AddResource([]byte(xmlValueFormat), "myProcess.bpmn").Send(ctx)
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
	request, err := zbClient.NewCreateInstanceCommand().BPMNProcessId(response.GetProcesses()[0].GetBpmnProcessId()).LatestVersion().VariablesFromMap(variables)
	if err != nil {
		return fmt.Errorf("流程实例创建失败:%s", err.Error())
	}

	_, err = request.Send(ctx)
	if err != nil {
		return fmt.Errorf("流程推进到下一阶段失败:%s", err.Error())
	}
	return nil
}
