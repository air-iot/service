package mongo

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var Client *mongo.Client
var Database *mongo.Database
var DB = "iot"

// Init 初始化mongo客户端
func Init() {
	if !viper.GetBool("mongo.enable") {
		return
	}
	var (
		username = viper.GetString("mongo.username")
		password = viper.GetString("mongo.password")
		host     = viper.GetString("mongo.host")
		port     = viper.GetInt("mongo.port")
		db       = viper.GetString("mongo.db")
		adminDB  = viper.GetString("mongo.adminDb")
		poolSize = viper.GetInt("mongo.poolSize")
	)
	if adminDB == "" {
		adminDB = "admin"
	}
	if db != "" {
		DB = db
	}
	if username == "" {
		username = "root"
	}
	if password == "" {
		password = "dell123"
	}
	if host == "" {
		host = "mongo"
	}
	if port == 0 {
		port = 27017
	}
	if poolSize == 0 {
		poolSize = 10
	}
	var err error
	opts := options.Client().ApplyURI(fmt.Sprintf("mongodb://%s:%s@%s:%d/%s",
		username,
		password,
		host,
		port,
		adminDB,
	)).SetMaxPoolSize(uint64(poolSize)).SetHeartbeatInterval(30 * time.Second).SetMaxConnIdleTime(30 * time.Second)
	Client, err = mongo.NewClient(opts)
	if err != nil {
		logrus.Panic(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err = Client.Connect(ctx)
	if err != nil {
		logrus.Panic(err)

	}
	Database = Client.Database(DB)
}

func Close() {
	if Client != nil {
		err := Client.Disconnect(context.Background())
		if err != nil {
			logrus.Errorf("Mongo关闭失败:%s", err.Error())
		}
	}
}
