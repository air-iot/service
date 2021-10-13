package api

import (
	"net/url"
	"time"
)

type AuthToken struct {
	TokenType   string `json:"tokenType"`
	ExpiresAt   int64  `json:"expiresAt"`
	AccessToken string `json:"accessToken"`
}

type Client interface {
	FindToken(project string) (*AuthToken, error)

	Get(url url.URL, headers map[string]string, result interface{}) error
	Post(url url.URL, headers map[string]string, data, result interface{}) error
	Delete(url url.URL, headers map[string]string, result interface{}) error
	Put(url url.URL, headers map[string]string, data, result interface{}) error
	Patch(url url.URL, headers map[string]string, data, result interface{}) error

	GetLatest(headers map[string]string, query, result interface{}) error
	PostLatest(headers map[string]string, data, result interface{}) error
	GetQuery(headers map[string]string, query, result interface{}) error
	PostQuery(headers map[string]string, query, result interface{}) error

	FindModelQuery(headers map[string]string, query, result interface{}) error
	FindModelById(headers map[string]string, id string, result interface{}) error
	SaveModel(headers map[string]string, data, result interface{}) error
	DelModelById(headers map[string]string, id string, result interface{}) error
	UpdateModelById(headers map[string]string, id string, data, result interface{}) error
	ReplaceModelById(headers map[string]string, id string, data, result interface{}) error

	FindNodeQuery(headers map[string]string, query, result interface{}) error
	FindNodeById(headers map[string]string, id string, result interface{}) error
	SaveNode(headers map[string]string, data, result interface{}) error
	DelNodeById(headers map[string]string, id string, result interface{}) error
	UpdateNodeById(headers map[string]string, id string, data, result interface{}) error
	ReplaceNodeById(headers map[string]string, id string, data, result interface{}) error
	FindTagsById(headers map[string]string, id string, result interface{}) error

	FindUserQuery(headers map[string]string, query, result interface{}) error
	FindUserById(headers map[string]string, id string, result interface{}) error

	SaveUser(headers map[string]string, data, result interface{}) error
	DelUserById(headers map[string]string, id string, result interface{}) error
	UpdateUserById(headers map[string]string, id string, data, result interface{}) error
	ReplaceUserById(headers map[string]string, id string, data, result interface{}) error

	FindHandlerQuery(headers map[string]string, query, result interface{}) error
	FindHandlerById(headers map[string]string, id string, result interface{}) error
	SaveHandler(headers map[string]string, data, result interface{}) error
	DelHandlerById(headers map[string]string, id string, result interface{}) error
	UpdateHandlerById(headers map[string]string, id string, data, result interface{}) error
	ReplaceHandlerById(headers map[string]string, id string, data, result interface{}) error

	FindExtQuery(headers map[string]string, collection string, query, result interface{}) error
	FindExtById(headers map[string]string, collection, id string, result interface{}) error
	SaveExt(headers map[string]string, collection string, data, result interface{}) error
	SaveManyExt(headers map[string]string, collection string, data, result interface{}) error
	DelExtById(headers map[string]string, collection, id string, result interface{}) error
	UpdateExtById(headers map[string]string, collection, id string, data, result interface{}) error
	UpdateManyExt(headers map[string]string, collection string, query, data, result interface{}) error
	ReplaceExtById(headers map[string]string, collection, id string, data, result interface{}) error
	DelExtAll(headers map[string]string, tableName, result interface{}) error
	DelManyExt(headers map[string]string, collection string, query, result interface{}) error

	FindEventQuery(headers map[string]string, query, result interface{}) error
	FindEventById(headers map[string]string, id string, result interface{}) error
	SaveEvent(headers map[string]string, data, result interface{}) error
	DelEventById(headers map[string]string, id string, result interface{}) error
	UpdateEventById(headers map[string]string, id string, data, result interface{}) error
	ReplaceEventById(headers map[string]string, id string, data, result interface{}) error

	FindSettingQuery(headers map[string]string, query, result interface{}) error
	FindSettingById(headers map[string]string, id string, result interface{}) error
	SaveSetting(headers map[string]string, data, result interface{}) error
	DelSettingById(headers map[string]string, id string, result interface{}) error
	UpdateSettingById(headers map[string]string, id string, data, result interface{}) error
	ReplaceSettingById(headers map[string]string, id string, data, result interface{}) error

	FindTableQuery(headers map[string]string, query, result interface{}) error
	FindTableById(headers map[string]string, id string, result interface{}) error
	SaveTable(headers map[string]string, data, result interface{}) error
	DelTableById(headers map[string]string, id string, result interface{}) error
	UpdateTableById(headers map[string]string, id string, data, result interface{}) error
	ReplaceTableById(headers map[string]string, id string, data, result interface{}) error

	FindWarnQuery(headers map[string]string, archive bool, query, result interface{}) error
	FindWarnById(headers map[string]string, id string, archive bool, result interface{}) error
	SaveWarn(headers map[string]string, data interface{}, archive bool, result interface{}) error
	DelWarnById(headers map[string]string, id string, archive bool, result interface{}) error
	UpdateWarnById(headers map[string]string, id string, archive bool, data, result interface{}) error
	ReplaceWarnById(headers map[string]string, id string, archive bool, data, result interface{}) error

	ChangeCommand(headers map[string]string, id string, data, result interface{}) error
	DriverConfig(headers map[string]string, driverId, serviceId string) ([]byte, error)

	FindGatewayQuery(headers map[string]string, query, result interface{}) error
	FindGatewayById(headers map[string]string, id string, result interface{}) error
	FindGatewayByType(headers map[string]string, typeName string, result interface{}) error
	SaveGateway(headers map[string]string, data, result interface{}) error
	DelGatewayById(headers map[string]string, id string, result interface{}) error
	UpdateGatewayById(headers map[string]string, id string, data, result interface{}) error
	ReplaceGatewayById(headers map[string]string, id string, data, result interface{}) error

	CheckDriver(headers map[string]string, licenseName string, signature interface{}) error

	FindLogQuery(headers map[string]string, query, result interface{}) error
	FindLogById(headers map[string]string, id string, result interface{}) error
	SaveLog(headers map[string]string, data, result interface{}) error
	DelLogById(headers map[string]string, id string, result interface{}) error
	UpdateLogById(headers map[string]string, id string, data, result interface{}) error
	ReplaceLogById(headers map[string]string, id string, data, result interface{}) error

	FindProjectQuery(headers map[string]string, timeout time.Duration, query, result interface{}) (int, error)

	FindFlowQuery(headers map[string]string, query, result interface{}) error
	FindFlowById(headers map[string]string, id string, result interface{}) error
	SaveFlow(headers map[string]string, data, result interface{}) error
	DelFlowById(headers map[string]string, id string, result interface{}) error
	UpdateFlowById(headers map[string]string, id string, data, result interface{}) error
	ReplaceFlowById(headers map[string]string, id string, data, result interface{}) error

	FindSystemVariableQuery(headers map[string]string, query, result interface{}) error
	FindSystemVariableById(headers map[string]string, id string, result interface{}) error
	SaveSystemVariable(headers map[string]string, data, result interface{}) error
	DelSystemVariableById(headers map[string]string, id string, result interface{}) error
	UpdateSystemVariableById(headers map[string]string, id string, data, result interface{}) error
	ReplaceSystemVariableById(headers map[string]string, id string, data, result interface{}) error

	FindDepartmentById(headers map[string]string, id string, result interface{}) error

	FindFlowTaskQuery(headers map[string]string, query, result interface{}) error
	FindFlowTaskById(headers map[string]string, id string, result interface{}) (int, error)
	SaveFlowTask(headers map[string]string, data, result interface{}) error
	DelFlowTaskById(headers map[string]string, id string, result interface{}) error
	UpdateFlowTaskById(headers map[string]string, id string, data, result interface{}) error
	ReplaceFlowTaskById(headers map[string]string, id string, data, result interface{}) error
}
