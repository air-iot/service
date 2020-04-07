package tools

func GetCronExpression(scheduleType string, data map[string]interface{}) string {
	cronExpression := ""
	switch scheduleType {
	case "hour":
		//minute, ok := data["minute"].(float64)
		//if !ok {
		//	minute = 0
		//}
		minuteString := InterfaceTypeToString(data["minute"])
		if minuteString == ""{
			minuteString = "0"
		}
		secondString := InterfaceTypeToString(data["second"])
		if secondString == ""{
			secondString = "0"
		}
		//second, ok := data["second"].(float64)
		//if !ok {
		//	second = 0
		//}
		//minuteString := strconv.Itoa(int(minute))
		//secondString := strconv.Itoa(int(second))
		cronExpression = secondString + " " + minuteString + " 0/1 * * ?"
	case "day":
		//hour, ok := data["hour"].(float64)
		//if !ok {
		//	hour = 0
		//}
		//minute, ok := data["minute"].(float64)
		//if !ok {
		//	minute = 0
		//}
		//second, ok := data["second"].(float64)
		//if !ok {
		//	second = 0
		//}
		//hourString := strconv.Itoa(int(hour))
		//minuteString := strconv.Itoa(int(minute))
		//secondString := strconv.Itoa(int(second))
		minuteString := InterfaceTypeToString(data["minute"])
		if minuteString == ""{
			minuteString = "0"
		}
		secondString := InterfaceTypeToString(data["second"])
		if secondString == ""{
			secondString = "0"
		}
		hourString := InterfaceTypeToString(data["hour"])
		if hourString == ""{
			hourString = "0"
		}
		cronExpression = secondString + " " + minuteString + " " + hourString + " * * ?"
	case "week":
		//week, ok := data["week"].(float64)
		//if !ok {
		//	week = 0
		//}
		//hour, ok := data["hour"].(float64)
		//if !ok {
		//	hour = 0
		//}
		//minute, ok := data["minute"].(int)
		//if !ok {
		//	minute = 0
		//}
		//second, ok := data["second"].(int)
		//if !ok {
		//	second = 0
		//}
		//weekString := strconv.Itoa(int(week))
		//hourString := strconv.Itoa(int(hour))
		//minuteString := strconv.Itoa(int(minute))
		//secondString := strconv.Itoa(int(second))
		minuteString := InterfaceTypeToString(data["minute"])
		if minuteString == ""{
			minuteString = "0"
		}
		secondString := InterfaceTypeToString(data["second"])
		if secondString == ""{
			secondString = "0"
		}
		hourString := InterfaceTypeToString(data["hour"])
		if hourString == ""{
			hourString = "0"
		}
		weekString := InterfaceTypeToString(data["week"])
		if weekString == ""{
			weekString = "1"
		}
		cronExpression = secondString + " " + minuteString + " " + hourString + " ? * "+weekString
	case "month":
		//day, ok := data["day"].(int)
		//if !ok {
		//	day = 0
		//}
		//hour, ok := data["hour"].(int)
		//if !ok {
		//	hour = 0
		//}
		//minute, ok := data["minute"].(int)
		//if !ok {
		//	minute = 0
		//}
		//second, ok := data["second"].(int)
		//if !ok {
		//	second = 0
		//}
		//dayString := strconv.Itoa(int(day))
		//hourString := strconv.Itoa(int(hour))
		//minuteString := strconv.Itoa(int(minute))
		//secondString := strconv.Itoa(int(second))
		dayString := InterfaceTypeToString(data["day"])
		if dayString == ""{
			dayString = "1"
		}
		minuteString := InterfaceTypeToString(data["minute"])
		if minuteString == ""{
			minuteString = "0"
		}
		secondString := InterfaceTypeToString(data["second"])
		if secondString == ""{
			secondString = "0"
		}
		hourString := InterfaceTypeToString(data["hour"])
		if hourString == ""{
			hourString = "0"
		}
		cronExpression = secondString + " " + minuteString + " " + hourString + " "+dayString+ " * ?"
	case "year":
		//month, ok := data["month"].(int)
		//if !ok {
		//	month = 0
		//}
		//day, ok := data["day"].(int)
		//if !ok {
		//	day = 0
		//}
		//hour, ok := data["hour"].(int)
		//if !ok {
		//	hour = 0
		//}
		//minute, ok := data["minute"].(int)
		//if !ok {
		//	minute = 0
		//}
		//second, ok := data["second"].(int)
		//if !ok {
		//	second = 0
		//}
		//monthString := strconv.Itoa(int(month))
		//dayString := strconv.Itoa(int(day))
		//hourString := strconv.Itoa(int(hour))
		//minuteString := strconv.Itoa(int(minute))
		//secondString := strconv.Itoa(int(second))
		monthString := InterfaceTypeToString(data["month"])
		if monthString == ""{
			monthString = "1"
		}
		dayString := InterfaceTypeToString(data["day"])
		if dayString == ""{
			dayString = "1"
		}
		minuteString := InterfaceTypeToString(data["minute"])
		if minuteString == ""{
			minuteString = "0"
		}
		secondString := InterfaceTypeToString(data["second"])
		if secondString == ""{
			secondString = "0"
		}
		hourString := InterfaceTypeToString(data["hour"])
		if hourString == ""{
			hourString = "0"
		}
		cronExpression = secondString + " " + minuteString + " " + hourString + " "+dayString+ " " +monthString +" ?"
	case "once":
		//month, ok := data["month"].(int)
		//if !ok {
		//	month = 0
		//}
		//day, ok := data["day"].(int)
		//if !ok {
		//	day = 0
		//}
		//hour, ok := data["hour"].(int)
		//if !ok {
		//	hour = 0
		//}
		//minute, ok := data["minute"].(int)
		//if !ok {
		//	minute = 0
		//}
		//second, ok := data["second"].(int)
		//if !ok {
		//	second = 0
		//}
		//monthString := strconv.Itoa(int(month))
		//dayString := strconv.Itoa(int(day))
		//hourString := strconv.Itoa(int(hour))
		//minuteString := strconv.Itoa(int(minute))
		//secondString := strconv.Itoa(int(second))
		monthString := InterfaceTypeToString(data["month"])
		if monthString == ""{
			monthString = "1"
		}
		dayString := InterfaceTypeToString(data["day"])
		if dayString == ""{
			dayString = "1"
		}
		minuteString := InterfaceTypeToString(data["minute"])
		if minuteString == ""{
			minuteString = "0"
		}
		secondString := InterfaceTypeToString(data["second"])
		if secondString == ""{
			secondString = "0"
		}
		hourString := InterfaceTypeToString(data["hour"])
		if hourString == ""{
			hourString = "0"
		}
		cronExpression = secondString + " " + minuteString + " " + hourString + " "+dayString+ " " +monthString +" ?"
	default:
	}
	return cronExpression
}
