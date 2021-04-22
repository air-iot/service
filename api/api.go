package api

import (
	"net/url"
)

type AuthToken struct {
	TokenType   string `json:"tokenType"`
	ExpiresAt   int64  `json:"expiresAt"`
	AccessToken string `json:"accessToken"`
}

type Client interface {
	Get(url url.URL, headers map[string]string, result interface{}) error
	Post(url url.URL, headers map[string]string, data, result interface{}) error
	Delete(url url.URL, headers map[string]string, result interface{}) error
	Put(url url.URL, headers map[string]string, data, result interface{}) error
	Patch(url url.URL, headers map[string]string, data, result interface{}) error

	GetLatest(query, result interface{}) error
	PostLatest(data, result interface{}) error
	GetQuery(query, result interface{}) error
	PostQuery(query, result interface{}) error

	FindModelQuery(query, result interface{}) error
	FindModelById(id string, result interface{}) error
	SaveModel(data, result interface{}) error
	DelModelById(id string, result interface{}) error
	UpdateModelById(id string, data, result interface{}) error
	ReplaceModelById(id string, data, result interface{}) error

	FindNodeQuery(query, result interface{}) error
	FindNodeById(id string, result interface{}) error
	SaveNode(data, result interface{}) error
	DelNodeById(id string, result interface{}) error
	UpdateNodeById(id string, data, result interface{}) error
	ReplaceNodeById(id string, data, result interface{}) error
	FindTagsById(id string, result interface{}) error

	FindUserQuery(query, result interface{}) error
	FindUserById(id string, result interface{}) error
	SaveUser(data, result interface{}) error
	DelUserById(id string, result interface{}) error
	UpdateUserById(id string, data, result interface{}) error
	ReplaceUserById(id string, data, result interface{}) error

	FindHandlerQuery(query, result interface{}) error
	FindHandlerById(id string, result interface{}) error
	SaveHandler(data, result interface{}) error
	DelHandlerById(id string, result interface{}) error
	UpdateHandlerById(id string, data, result interface{}) error
	ReplaceHandlerById(id string, data, result interface{}) error

	FindExtQuery(collection string, query, result interface{}) error
	FindExtById(collection, id string, result interface{}) error
	SaveExt(collection string, data, result interface{}) error
	SaveManyExt(collection string, data, result interface{}) error
	DelExtById(collection, id string, result interface{}) error
	UpdateExtById(collection, id string, data, result interface{}) error
	ReplaceExtById(collection, id string, data, result interface{}) error
	DelExtAll(tableName, result interface{}) error

	FindEventQuery(query, result interface{}) error
	FindEventById(id string, result interface{}) error
	SaveEvent(data, result interface{}) error
	DelEventById(id string, result interface{}) error
	UpdateEventById(id string, data, result interface{}) error
	ReplaceEventById(id string, data, result interface{}) error

	FindSettingQuery(query, result interface{}) error
	FindSettingById(id string, result interface{}) error
	SaveSetting(data, result interface{}) error
	DelSettingById(id string, result interface{}) error
	UpdateSettingById(id string, data, result interface{}) error
	ReplaceSettingById(id string, data, result interface{}) error

	FindTableQuery(query, result interface{}) error
	FindTableById(id string, result interface{}) error
	SaveTable(data, result interface{}) error
	DelTableById(id string, result interface{}) error
	UpdateTableById(id string, data, result interface{}) error
	ReplaceTableById(id string, data, result interface{}) error

	FindWarnQuery(archive bool, query, result interface{}) error
	FindWarnById(id string, archive bool, result interface{}) error
	SaveWarn(data, archive bool, result interface{}) error
	DelWarnById(id string, archive bool, result interface{}) error
	UpdateWarnById(id string, archive bool, data, result interface{}) error
	ReplaceWarnById(id string, archive bool, data, result interface{}) error

	ChangeCommand(id string, data, result interface{}) error
	DriverConfig(driverId, serviceId string) ([]byte, error)

	FindGatewayQuery(query, result interface{}) error
	FindGatewayById(id string, result interface{}) error
	FindGatewayByType(typeName string, result interface{}) error
	SaveGateway(data, result interface{}) error
	DelGatewayById(id string, result interface{}) error
	UpdateGatewayById(id string, data, result interface{}) error
	ReplaceGatewayById(id string, data, result interface{}) error

	// license
	//CheckDriver(licenseName string) (*model.Signature, error)

	FindLogQuery(query, result interface{}) error
	FindLogById(id string, result interface{}) error
	SaveLog(data, result interface{}) error
	DelLogById(id string, result interface{}) error
	UpdateLogById(id string, data, result interface{}) error
	ReplaceLogById(id string, data, result interface{}) error
}
