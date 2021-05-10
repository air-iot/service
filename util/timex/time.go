package timex

import (
	"regexp"
	"strconv"
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

func IsTimeGroup(data string) bool {
	pattern, _ := regexp.Compile("^time\\(\\d+(s|m|h|d|w|mo|y)\\)$")
	return pattern.MatchString(data)
}

func GetInfluxOffset(startTime, intervalSecond int64) int64 {
	epoch := time.Unix(0, 0).Unix()
	offset := (startTime - epoch) % intervalSecond
	return offset
}

func GetFutureMonthTime(referTime *time.Time, i int) *time.Time {
	month := referTime.Month()

	newMonth := int(month) + i
	t := time.Date(referTime.Year(), time.Month(newMonth), referTime.Day(), referTime.Hour(), referTime.Minute(), referTime.Second(), 0, time.Local)
	return &t
}

func GetFutureYearTime(referTime *time.Time, i int) *time.Time {
	year := referTime.Year()

	newYear := int(year) + i
	t := time.Date(newYear, referTime.Month(), referTime.Day(), referTime.Hour(), referTime.Minute(), referTime.Second(), 0, time.Local)
	return &t
}

func IsSameDay(referTime, endTime *time.Time) bool {
	if referTime.Year() == endTime.Year() &&
		referTime.Month() == endTime.Month() &&
		referTime.Day() == endTime.Day() {
		return true
	}
	return false
}
