package tools

import "time"

func GetCronExpression(scheduleType string, data map[string]interface{}) string {
	cronExpression := ""
	switch scheduleType {
	case "second":
		secondString := InterfaceTypeToString(data["second"])
		if secondString == "" {
			secondString = "0"
		}
		cronExpression = "0/" + secondString + " * * * * ?"
	case "minute":
		minuteString := InterfaceTypeToString(data["minute"])
		if minuteString == "" {
			minuteString = "0"
		}
		secondString := InterfaceTypeToString(data["second"])
		if secondString == "" {
			secondString = "0"
		}
		cronExpression = secondString + " 0/" + minuteString + " * * * ?"
	case "hour":
		hourString := InterfaceTypeToString(data["hour"])
		if hourString == "" {
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
		cronExpression = secondString + " " + minuteString + " " + hourString + " 0/" + dayString + " * ?"
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
		cronExpression = secondString + " " + minuteString + " " + hourString + " " + dayString + " 0/" + monthString + " ?"
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

//func GetCronExpressionBak(scheduleType string, data map[string]interface{}) string {
//	cronExpression := ""
//	switch scheduleType {
//	case "hour":
//		minuteString := InterfaceTypeToString(data["minute"])
//		if minuteString == "" {
//			minuteString = "0"
//		}
//		secondString := InterfaceTypeToString(data["second"])
//		if secondString == "" {
//			secondString = "0"
//		}
//		cronExpression = secondString + " " + minuteString + " 0/1 * * ?"
//	case "day":
//		minuteString := InterfaceTypeToString(data["minute"])
//		if minuteString == "" {
//			minuteString = "0"
//		}
//		secondString := InterfaceTypeToString(data["second"])
//		if secondString == "" {
//			secondString = "0"
//		}
//		hourString := InterfaceTypeToString(data["hour"])
//		if hourString == "" {
//			hourString = "0"
//		}
//		cronExpression = secondString + " " + minuteString + " " + hourString + " * * ?"
//	case "week":
//		minuteString := InterfaceTypeToString(data["minute"])
//		if minuteString == "" {
//			minuteString = "0"
//		}
//		secondString := InterfaceTypeToString(data["second"])
//		if secondString == "" {
//			secondString = "0"
//		}
//		hourString := InterfaceTypeToString(data["hour"])
//		if hourString == "" {
//			hourString = "0"
//		}
//		weekString := InterfaceTypeToString(data["week"])
//		if weekString == "" {
//			weekString = "1"
//		}
//		cronExpression = secondString + " " + minuteString + " " + hourString + " ? * " + weekString
//	case "month":
//		dayString := InterfaceTypeToString(data["day"])
//		if dayString == "" {
//			dayString = "1"
//		}
//		minuteString := InterfaceTypeToString(data["minute"])
//		if minuteString == "" {
//			minuteString = "0"
//		}
//		secondString := InterfaceTypeToString(data["second"])
//		if secondString == "" {
//			secondString = "0"
//		}
//		hourString := InterfaceTypeToString(data["hour"])
//		if hourString == "" {
//			hourString = "0"
//		}
//		cronExpression = secondString + " " + minuteString + " " + hourString + " " + dayString + " * ?"
//	case "year":
//		monthString := InterfaceTypeToString(data["month"])
//		if monthString == "" {
//			monthString = "1"
//		}
//		dayString := InterfaceTypeToString(data["day"])
//		if dayString == "" {
//			dayString = "1"
//		}
//		minuteString := InterfaceTypeToString(data["minute"])
//		if minuteString == "" {
//			minuteString = "0"
//		}
//		secondString := InterfaceTypeToString(data["second"])
//		if secondString == "" {
//			secondString = "0"
//		}
//		hourString := InterfaceTypeToString(data["hour"])
//		if hourString == "" {
//			hourString = "0"
//		}
//		cronExpression = secondString + " " + minuteString + " " + hourString + " " + dayString + " " + monthString + " ?"
//	case "once":
//		monthString := InterfaceTypeToString(data["month"])
//		if monthString == "" {
//			monthString = "1"
//		}
//		dayString := InterfaceTypeToString(data["day"])
//		if dayString == "" {
//			dayString = "1"
//		}
//		minuteString := InterfaceTypeToString(data["minute"])
//		if minuteString == "" {
//			minuteString = "0"
//		}
//		secondString := InterfaceTypeToString(data["second"])
//		if secondString == "" {
//			secondString = "0"
//		}
//		hourString := InterfaceTypeToString(data["hour"])
//		if hourString == "" {
//			hourString = "0"
//		}
//		cronExpression = secondString + " " + minuteString + " " + hourString + " " + dayString + " " + monthString + " ?"
//	default:
//	}
//	return cronExpression
//}

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
