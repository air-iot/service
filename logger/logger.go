package logger

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"os"
)

const (
	MONGOPOOLGETERROR  = "从MongoDB连接池中获取连接失败: %s"
	REDISPOOLGETERROR  = "从Redis连接池中获取连接失败: %s"
	EMQTTPOOLGETERROR  = "从Mqtt连接池中获取连接失败: %s"
	INFLUXPOOLGETERROR = "从InfluxDB连接池中获取连接失败: %s"

	MONGOPOOLRELEASEERROR  = "释放MongoDB连接失败: %s"
	REDISPOOLRELEASEERROR  = "释放Redis连接失败: %s"
	EMQTTPOOLRELEASEERROR  = "释放Mqtt连接失败: %s"
	INFLUXPOOLRELEASEERROR = "释放InfluxDB连接失败: %s"

	QUERYERROR      = "根据过滤器查询数据失败: %s"
	QUERYBYIDERROR  = "根据id查询数据失败: %s"
	SAVEONEERROR    = "保存一条数据到数据库失败: %s"
	DELETEBYIDERROR = "根据id从数据库删除一条数据失败: %s"
	UPDATEBYIDERROR = "根据id更新数据失败: %s"

	INPUTUNMARSHALERROR   = "传入参数解序列化错误: %s"
	USERCACHEUPDATEERROR  = "更新用户权限缓存失败: %s"
	UPDATETAGMAPPINGERROR = "更新动态属性反向映射Map失败: %s"
)

func Init() {
	var tmpLogLevel = viper.GetString("log.level")
	var file = viper.GetString("log.file")
	l, err := logrus.ParseLevel(tmpLogLevel)
	if err != nil {
		l = logrus.ErrorLevel
	}
	// logrus.SetFormatter(&logrus.JSONFormatter{TimestampFormat: "2006-01-02 15:04:05"})
	if file != "" {
		err := os.MkdirAll("logs", 0666)
		if err != nil {
			panic(fmt.Sprintf("log dir: %s", err.Error()))
		}
		f, err := os.OpenFile("logs/"+file, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0666)
		if err != nil {
			panic(fmt.Sprintf("log: %s", err.Error()))
		}
		logrus.SetOutput(f)
	} else {
		logrus.SetOutput(os.Stdout)
	}
	logrus.SetLevel(l)

}

// Debugln 调试输出
func Debugln(fields map[string]interface{}, args ...interface{}) {
	if fields == nil {
		logrus.Debugln(args...)
	} else {
		logrus.WithFields(fields).Debugln(args...)
	}
}

// Debugf 调试输出
func Debugf(fields map[string]interface{}, format string, args ...interface{}) {
	if fields == nil {
		logrus.Debugf(format, args...)
	} else {
		logrus.WithFields(fields).Debugf(format, args...)
	}
}

// Infoln 信息输出
func Infoln(fields map[string]interface{}, args ...interface{}) {
	if fields == nil {
		logrus.Infoln(args...)
	} else {
		logrus.WithFields(fields).Infoln(args...)
	}
}

// Infof 信息输出
func Infof(fields map[string]interface{}, format string, args ...interface{}) {
	if fields == nil {
		logrus.Infof(format, args...)
	} else {
		logrus.WithFields(fields).Infof(format, args...)
	}
}

// Warnln 告警输出
func Warnln(fields map[string]interface{}, args ...interface{}) {
	if fields == nil {
		logrus.Warnln(args...)
	} else {
		logrus.WithFields(fields).Warnln(args...)
	}
}

// Warnf 告警输出
func Warnf(fields map[string]interface{}, format string, args ...interface{}) {
	if fields == nil {
		logrus.Warnf(format, args...)
	} else {
		logrus.WithFields(fields).Warnf(format, args...)
	}
}

// Errorln 错误输出
func Errorln(fields map[string]interface{}, args ...interface{}) {
	if fields == nil {
		logrus.Errorln(args...)
	} else {
		logrus.WithFields(fields).Errorln(args...)
	}
}

// Errorf 错误输出
func Errorf(fields map[string]interface{}, format string, args ...interface{}) {
	if fields == nil {
		logrus.Errorf(format, args...)
	} else {
		logrus.WithFields(fields).Errorf(format, args...)
	}
}
