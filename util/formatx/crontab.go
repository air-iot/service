package formatx

import (
	"github.com/air-iot/service/util/numberx"
	"strings"
	"time"
)

func GetCronExpression(scheduleType string, data map[string]interface{}) string {
	cronExpression := ""
	numberInterval := numberx.HasNumberExp(scheduleType)
	if len(numberInterval) != 0{
		scheduleType = strings.ReplaceAll(scheduleType,numberInterval,"")
	}
	switch scheduleType {
	case "second":
		if len(numberInterval) != 0{
			data["second"] = numberInterval
		}
		secondString := InterfaceTypeToString(data["second"])
		if secondString == "" {
			secondString = "1"
		}
		cronExpression = "0/" + secondString + " * * * * ?"
	case "minute":
		if len(numberInterval) != 0{
			data["minute"] = numberInterval
		}
		minuteString := InterfaceTypeToString(data["minute"])
		if minuteString == "" || minuteString == "0" {
			minuteString = "1"
		}
		secondString := InterfaceTypeToString(data["second"])
		if secondString == "" {
			secondString = "0"
		}
		cronExpression = secondString + " 0/" + minuteString + " * * * ?"
	case "hour":
		if len(numberInterval) != 0{
			data["hour"] = numberInterval
		}
		hourString := InterfaceTypeToString(data["hour"])
		if hourString == "" || hourString == "0" {
			hourString = "1"
		}
		minuteString := InterfaceTypeToString(data["minute"])
		if minuteString == "" {
			minuteString = "0"
		}
		secondString := InterfaceTypeToString(data["second"])
		if secondString == "" {
			secondString = "0"
		}
		cronExpression = secondString + " " + minuteString + " 0/" + hourString + " * * ?"
	case "day":
		if len(numberInterval) != 0{
			data["day"] = numberInterval
		}
		dayString := InterfaceTypeToString(data["day"])
		if dayString == "" || dayString == "0" {
			dayString = "1"
		}
		minuteString := InterfaceTypeToString(data["minute"])
		if minuteString == "" {
			minuteString = "0"
		}
		secondString := InterfaceTypeToString(data["second"])
		if secondString == "" {
			secondString = "0"
		}
		hourString := InterfaceTypeToString(data["hour"])
		if hourString == "" {
			hourString = "0"
		}
		cronExpression = secondString + " " + minuteString + " " + hourString + " 1/" + dayString + " * ?"
	case "week":
		minuteString := InterfaceTypeToString(data["minute"])
		if minuteString == "" {
			minuteString = "0"
		}
		secondString := InterfaceTypeToString(data["second"])
		if secondString == "" {
			secondString = "0"
		}
		hourString := InterfaceTypeToString(data["hour"])
		if hourString == "" {
			hourString = "0"
		}
		weekString := InterfaceTypeToString(data["week"])
		if weekString == "" {
			weekString = "1"
		}
		cronExpression = secondString + " " + minuteString + " " + hourString + " ? * " + weekString
	case "month":
		if len(numberInterval) != 0{
			data["month"] = numberInterval
		}
		monthString := InterfaceTypeToString(data["month"])
		if monthString == "" || monthString == "0" {
			monthString = "1"
		}
		dayString := InterfaceTypeToString(data["day"])
		if dayString == "" {
			dayString = "1"
		}
		minuteString := InterfaceTypeToString(data["minute"])
		if minuteString == "" {
			minuteString = "0"
		}
		secondString := InterfaceTypeToString(data["second"])
		if secondString == "" {
			secondString = "0"
		}
		hourString := InterfaceTypeToString(data["hour"])
		if hourString == "" {
			hourString = "0"
		}
		cronExpression = secondString + " " + minuteString + " " + hourString + " " + dayString + " 1/" + monthString + " ?"
	case "year":
		monthString := InterfaceTypeToString(data["month"])
		if monthString == "" {
			monthString = "1"
		}
		dayString := InterfaceTypeToString(data["day"])
		if dayString == "" {
			dayString = "1"
		}
		minuteString := InterfaceTypeToString(data["minute"])
		if minuteString == "" {
			minuteString = "0"
		}
		secondString := InterfaceTypeToString(data["second"])
		if secondString == "" {
			secondString = "0"
		}
		hourString := InterfaceTypeToString(data["hour"])
		if hourString == "" {
			hourString = "0"
		}
		cronExpression = secondString + " " + minuteString + " " + hourString + " " + dayString + " " + monthString + " ?"
	case "once":
		monthString := InterfaceTypeToString(data["month"])
		if monthString == "" {
			monthString = "1"
		}
		dayString := InterfaceTypeToString(data["day"])
		if dayString == "" {
			dayString = "1"
		}
		minuteString := InterfaceTypeToString(data["minute"])
		if minuteString == "" {
			minuteString = "0"
		}
		secondString := InterfaceTypeToString(data["second"])
		if secondString == "" {
			secondString = "0"
		}
		hourString := InterfaceTypeToString(data["hour"])
		if hourString == "" {
			hourString = "0"
		}
		cronExpression = secondString + " " + minuteString + " " + hourString + " " + dayString + " " + monthString + " ?"
	default:
	}
	return cronExpression
}

func GetCronExpressionOnce(scheduleType string, data *time.Time) string {
	cronExpression := ""
	monthString := InterfaceTypeToString(int(data.Month()))
	if monthString == "" {
		monthString = "1"
	}
	dayString := InterfaceTypeToString(data.Day())
	if dayString == "" {
		dayString = "1"
	}
	minuteString := InterfaceTypeToString(data.Minute())
	if minuteString == "" {
		minuteString = "0"
	}
	secondString := InterfaceTypeToString(data.Second())
	if secondString == "" {
		secondString = "0"
	}
	hourString := InterfaceTypeToString(data.Hour())
	if hourString == "" {
		hourString = "0"
	}
	cronExpression = secondString + " " + minuteString + " " + hourString + " " + dayString + " " + monthString + " ?"
	return cronExpression
}
