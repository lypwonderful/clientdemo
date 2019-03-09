package main

import (
	"github.com/lypwonderful/clientdemo/pkgs/core/dubbogo"
	"github.com/lypwonderful/clientdemo/pkgs/usercenter"
)

func main() {
	initClient()
	usercenter.GetUserCenterInfo()
	dubbogo.DubboClientRun()
}

func initClient() {
	dubbogo.InitDubbo()
}
