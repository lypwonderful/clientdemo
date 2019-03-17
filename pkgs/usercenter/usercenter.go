package usercenter

import (
	"fmt"
	"github.com/AlexStocks/goext/log"
	"github.com/AlexStocks/goext/time"
	"github.com/lypwonderful/clientdemo/pkgs/core/dubbogo"
	_ "net/http/pprof"
)

type JsonRPCUser struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Age  int64  `json:"age"`
	Time int64  `json:"time"`
	Sex  string `json:"sex"`
}
type Users struct {
	User []JsonRPCUser
}

func (u JsonRPCUser) String() string {
	return fmt.Sprintf(
		"User{ID:%s, Name:%s, Age:%d, Time:%s, Sex:%s}",
		u.ID, u.Name, u.Age, gxtime.YMDPrint(int(u.Time), 0), u.Sex,
	)
}

func GetUserCenterInfo() {
	userC := &dubbogo.DubboClient{
		ServiceName: "com.ikurento.user.UserProvider",
		Method:      "GetUserInfo",
		Args:        []string{"A003", "A001"},
	}

	uInfo := new(Users)
	err := userC.Prepare().ClientFunc(uInfo)
	if err != nil {
		panic(err)
	}
	gxlog.CInfo("user info:%+v", uInfo)
}
