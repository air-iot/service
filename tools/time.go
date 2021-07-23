package tools

import (
	"strconv"
	"strings"
	"time"
)

func mergeTimeString(dateString *string, timeUnit string, value int64) {
	if value != 0 {
		*dateString = strconv.Itoa(int(value)) + timeUnit
	}
}

//获取时间间隔转化为天小时分秒
func GetFormatTimeSpanString(span int64) string {
	var hour int64
	var minute int64

	day := span / (3600 * 24)

	if day > 0 {
		hour = (span / 3600) % 24
	} else {
		hour = span / 3600
	}

	if hour > 0 {
		minute = (span / 60) % 60
	} else {
		minute = span / 60
	}

	sec := span % 60

	dayString := ""
	hourString := ""
	minuteString := ""
	secString := ""

	mergeTimeString(&dayString, "天", day)
	mergeTimeString(&hourString, "小时", hour)
	mergeTimeString(&minuteString, "分", minute)
	mergeTimeString(&secString, "秒", sec)

	return dayString + hourString + minuteString + secString
}

//格式字符串转Time
//time.ParseInLocation("20060102150405","20180223100000",time.Local)
func ConvertStringToTime(layout, value string, location *time.Location) (*time.Time, error) {
	t, err := time.ParseInLocation(layout, value, location)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// GetLocalTime 获取本地时区的时间（东八）
func GetLocalTime(data time.Time, format string) string {
	cstZone := time.FixedZone("CST", 8*3600) // 东八
	return data.In(cstZone).Format(format)
}

// GetLocalTime 获取本地时区的时间（东八）
func GetLocalTimeNow(data time.Time) time.Time {
	cstZone := time.FixedZone("CST", 8*3600) // 东八
	return data.In(cstZone)
}

// GetLocalTime 获取本地时区的时间（东八）
func GetUTCTimeNow(data time.Time) time.Time {
	cstZone := time.FixedZone("UTC", 0) // 0时区
	return data.In(cstZone)
}

//循环中获取前几个小时时间
func GetLastServeralHoursFromZero(i int) time.Time {
	currentHour, _ := strconv.Atoi(time.Now().Format("15"))

	oldHour := currentHour - i
	t := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), oldHour, 0, 0, 0, time.Local)
	return t
}

//循环中获取前几分钟时间
func GetLastServeralMinuteFromZero(i int) time.Time {
	currentMinute := time.Now().Minute()
	oldMinute := currentMinute - i
	t := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), time.Now().Hour(), oldMinute, 0, 0, time.Local)
	return t
}

//前推到几天前0点
func GetUnixToOldTimeDay(i int) time.Time {
	day := time.Now().Day()

	oldMonth := day - i
	t := time.Date(time.Now().Year(), time.Now().Month(), oldMonth, 0, 0, 0, 0, time.Local)
	return t
}

//前推到几天后0点
func GetUnixToNewTimeDay(i int) time.Time {
	day := time.Now().Day()

	oldMonth := day + i
	t := time.Date(time.Now().Year(), time.Now().Month(), oldMonth, 0, 0, 0, 0, time.Local)
	return t
}

//几个月前的月初
func GetUnixToOldTime(i int) time.Time {
	currentMonth := int(time.Now().Month())

	oldMonth := currentMonth - i
	t := time.Date(time.Now().Year(), time.Month(oldMonth), 1, 0, 0, 0, 0, time.Local)
	return t
}

//后几个月的月初，指定时间
func GetUnixToNewTime(i int, timestamp int64) time.Time {
	currentMonth := int(time.Unix(timestamp, 0).Month())

	newMonth := currentMonth + i
	t := time.Date(time.Unix(timestamp, 0).Year(), time.Month(newMonth), 1, 0, 0, 0, 0, time.Local)
	return t
}

//几年前的当月初
func GetUnixToOldYearTime(i, month int) time.Time {
	currentYear := int(time.Now().Year())
	currentMonth := int(time.Now().Month())
	oldYear := currentYear - i
	newMonth := currentMonth - month
	t := time.Date(oldYear, time.Month(newMonth), 1, 0, 0, 0, 0, time.Local)
	return t
}

//几年前的季度
func GetUnixToOldYearQuarterTime(i, month int) time.Time {
	currentYear := int(time.Now().Year())
	oldYear := currentYear - i
	t := time.Date(oldYear, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	return t
}

func FormatTimeFormat(data string) string {
	formatLayout := "2006-01-02T15:04:05"
	if !strings.Contains(data,"T"){
		formatLayout = "2006-01-02 15:04:05"
	}
	if strings.Contains(data, "Z") {
		switch len(data) {
		case 20:
			formatLayout = formatLayout + "Z"
		case 21:
			formatLayout = formatLayout + ".0Z"
		case 22:
			formatLayout = formatLayout + ".00Z"
		case 23:
			formatLayout = formatLayout + ".000Z"
		}
	} else {
		switch len(data) {
		case 25:
			formatLayout = formatLayout + data[len(data)-6:]
		case 27:
			formatLayout = formatLayout + ".0" + data[len(data)-6:]
		case 28:
			formatLayout = formatLayout + ".00" + data[len(data)-6:]
		case 29:
			formatLayout = formatLayout + ".000" + data[len(data)-6:]
		}
	}
	return formatLayout
}
