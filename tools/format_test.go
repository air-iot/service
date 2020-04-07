package tools_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"gopkg.in/resty.v1"

	"github.com/air-iot/service/tools"
)

func TestAddNonRepByLoop(t *testing.T) {
	aList := []string{"1", "2"}
	elem := "2"
	fmt.Println(tools.AddNonRepByLoop(aList, elem))
	elem = "3"
	fmt.Println(tools.AddNonRepByLoop(aList, elem))
}

func TestNonKeyMap(t *testing.T) {
	m := map[string]interface{}{}
	_, ok := m["a"].(string)
	if !ok {
		fmt.Println("!ok")
	}
}

func TestSplit(t *testing.T) {
	firstURIList := strings.Split("/users/id123", "/")
	fmt.Println(len(firstURIList))
	firstURI := "/" + firstURIList[1]
	fmt.Println(firstURI)
	fmt.Println(strings.Title(firstURIList[1]))

	fmt.Printf("%v", generatePermissionMap(firstURIList[1]))
}

func TestSlice(t *testing.T) {
	a := make([]int, 0)
	for i := 0; i < 10; i++ {
		a = append(a, i)
	}
	fmt.Println("a before", a)
	b := a[:]
	b = append(b, 22)
	b[0] = 111
	fmt.Println("a after", a)
	fmt.Println("b", b)

	//fmt.Printf("%v", generatePermissionMap(firstURIList[1]))
}

func generatePermissionMap(data string) map[string]string {
	uriValue := ""
	if data[len(data)-1:] == "s" {
		uriValue = data[:len(data)-1]
	}
	uriValue = strings.Title(uriValue)
	return map[string]string{
		data + "+" + http.MethodGet:    uriValue + ".view",
		data + "+" + http.MethodPost:   uriValue + ".add",
		data + "+" + http.MethodPatch:  uriValue + ".edit",
		data + "+" + http.MethodPut:    uriValue + ".edit",
		data + "+" + http.MethodDelete: uriValue + ".delete",
	}
}

func TestTokenAndPermission(t *testing.T) {
	resp, err := resty.NewRequest().
		SetHeader("Authorization", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE1NTE2ODM4MjEsImlhdCI6MTU1MTY4MDIyMSwiaWQiOiI1Yzc0ZWRiYzZmNTUzZTRmY2E1ZGY5YzYifQ.EJLvFT9NYzAMpwKd_3nkNYNMri1Ox5pfb7sHMlkW0gs").
		Get("http://localhost:9000/users/getUserInfo")
	if err != nil {
		t.Fatal("请求失败:", err, "statuscode:", resp.StatusCode())
	}
	t.Logf("string(resp.Body()):%s", string(resp.Body()))
}

func TestNodePermission(t *testing.T) {
	//5c63cd1f93d9cc00ad2f4014
	//queryMap := map[string]interface{}{
	//	"filter": map[string]interface{}{
	//		"_id": "5c63cd1693d9cc00ad2f4010",
	//		"$and": []map[string]interface{}{
	//			{
	//				"_id": "5c63cd1693d9cc00ad2f4010",
	//			},
	//			{
	//				"_id": "5c63cd1693d9cc00ad2f4010",
	//			},
	//		},
	//	},
	//}
	//b, err := json.Marshal(queryMap)
	fmt.Println(int(time.Hour))
	fmt.Println(int(time.Hour * 365 * 24))
	fmt.Println(time.Hour * 365 * 24)

	resp, err := resty.NewRequest().
		SetHeader("Authorization", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE1ODM5MjMwNDMsImlhdCI6MTU1MjM4NzA0MywiaWQiOiI1Yzg3OGI4YzQxN2I5MDg5OWJjMTcwMTQifQ.Ov58zUHe5Fd5ipjJSCrZtE0lsqc_XrwsVTe6d4J3Rrs").
		//SetQueryParam("query", string(b)).
		Get("http://localhost:9000/model")
	if err != nil {
		t.Fatal("请求失败:", err, "statuscode:", resp.StatusCode())
	}
	t.Logf("string(resp.Body()):%s", string(resp.Body()))
	t.Logf("count:%s", resp.Header().Get("count"))
}

func TestWarningCombine(t *testing.T) {
	testData := `[
  {
    "id": 180,
    "name": "EP网专利检索",
    "input": "",
    "info": "",
    "homePage": "http://ea.espacenet.com/search97cgi/s97_cgi.exe?Action=FormGen=ea/RU/home.hts",
    "language": 2,
    "status": 1,
    "isFulltext": 0,
    "provider": "欧亚专利组织",
    "visit": 0,
    "search": 0
  },
  {
    "id": 181,
    "name": "专利检索",
    "input": "",
    "info": "",
    "homePage": "http://www.eapatis.com/",
    "language": 2,
    "status": 1,
    "isFulltext": 0,
    "provider": "欧亚专利组织",
    "visit": 0,
    "search": 0
  },
  {
    "id": 182,
    "name": "专利检索",
    "input": "",
    "info": "",
    "homePage": "http://ep.espacenet.com/",
    "language": 2,
    "status": 1,
    "isFulltext": 0,
    "provider": "欧洲专利组织",
    "visit": 0,
    "search": 0
  },
  {
    "id": 183,
    "name": "法律状态检索",
    "input": "",
    "info": "",
    "homePage": "http://www.epoline.org/portal/public/!ut/p/kcxml/04_Sj9SPykssy0xPLMnMz0vM0Y_QjzKLN4i3dAHJgFjGpvqRqCKOcAFvfV-P_NxU_QD9gtzQiHJHRUUA43OWZA!!/delta/base64xml/L3dJdyEvUUd3QndNQSEvNElVRS82XzBfOUc!",
    "language": 2,
    "status": 1,
    "isFulltext": 0,
    "provider": "欧洲专利组织",
    "visit": 0,
    "search": 0
  },
  {
    "id": 184,
    "name": "外观设计检索",
    "input": "",
    "info": "",
    "homePage": "http://oami.europa.eu/RCDOnline/RequestManager",
    "language": 2,
    "status": 1,
    "isFulltext": 0,
    "provider": "欧洲内部市场协调局（商标和外观设计）",
    "visit": 0,
    "search": 0
  },
  {
    "id": 185,
    "name": "外观设计公报（2003~）",
    "input": "",
    "info": "",
    "homePage": "http://oami.europa.eu/bulletin/rcd/rcd_bulletin_en.htm",
    "language": 2,
    "status": 1,
    "isFulltext": 0,
    "provider": "欧洲内部市场协调局（商标和外观设计）",
    "visit": 0,
    "search": 0
  },
  {
    "id": 186,
    "name": "外观设计公报222",
    "input": "",
    "info": "",
    "homePage": "http://oami.europa.eu/bulletin/rcd/rcd_bulletin_en.htm",
    "language": 2,
    "status": 1,
    "isFulltext": 0,
    "provider": "欧洲内部市场协调局（商标和外观设计）",
    "visit": 0,
    "search": 0
  }
]`

	//dataMap := make([]map[string]interface{}, 0)
	dataMap := make([]bson.M, 0)
	err := json.Unmarshal([]byte(testData), &dataMap)
	if err != nil {
		t.Fatalf("解序列化错误:%s", err)
	}

	//result := combineRecord(dataMap)
	result := combineRecordBson(dataMap)

	b, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("序列化错误:%s", err)
	}

	fmt.Println("result with maxRank:", string(b))

}

func TestEmptySlice(t *testing.T) {
	result := make([]bson.M, 0)
	fmt.Println(result[0])
}

func TestTimeZone(t *testing.T) {
	var cstZone = time.FixedZone("CST", 8*3600) // 东八
	fmt.Println("SH : ", time.Now().In(cstZone).Format("2006-01-02 15:04:05"))
}

func TestJPG(t *testing.T) {
	picFileRegexp, _ := regexp.Compile("(?i:html|htm|gif|jpg|jpeg|bmp|png|ico|txt|js|css|json|map)$")
	fmt.Println(picFileRegexp.MatchString("a.jPEG"))
}

func combineRecord(dataMap []map[string]interface{}) *[]map[string]interface{} {
	existMap := map[string]int{}
	for i, v := range dataMap {
		key := v["provider"].(string)
		if count, ok := existMap[key]; ok {
			existMap[key] = count + 1
			dataMap[i]["maxRank"] = 0
			dataMap[i-count]["maxRank"] = count + 1
		} else {
			existMap[key] = 1
			dataMap[i]["maxRank"] = 1
		}
	}

	return &dataMap
}

func combineRecordBson(dataMap []bson.M) *[]bson.M {
	existMap := map[string]int{}
	for i, v := range dataMap {
		key := v["provider"].(string)
		if count, ok := existMap[key]; ok {
			existMap[key] = count + 1
			dataMap[i]["maxRank"] = 0
			dataMap[i-count]["maxRank"] = count + 1
		} else {
			existMap[key] = 1
			dataMap[i]["maxRank"] = 1
		}
	}

	return &dataMap
}

//func TestRemoveNonExistEle(t *testing.T) {
//	list := []string{"a","b","c","d"}
//	filterList := []string{"a","b","w"}
//	removeNonExistEle()
//}
//
//func removeNonExistEle(list []string,filterList []string){
//
//}
