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
		adminDB  = viper.GetString("mongo.adminDb")
		poolSize = viper.GetInt("mongo.poolSize")
	)
	DB = viper.GetString("mongo.db")
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
		logrus.Fatalf("Mongo客户端创建错误: %s", err.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err = Client.Connect(ctx)
	if err != nil {
		logrus.Fatalf("Mongo客户端连接错误: %s", err.Error())
	}
	err = Client.Ping(context.Background(), nil)
	if err != nil {
		logrus.Fatalf("Mongo客户端 Ping 错误: %s", err.Error())
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
