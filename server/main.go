package main

import (
	// 内嵌时区数据库：飞牛等精简系统上未必装了 tzdata，业务时区固定北京时间，
	// 不能让 LoadLocation 因为缺少系统时区文件而失败。
	_ "time/tzdata"

	"smallgo/server/config"
	"smallgo/server/server"

	// Blank-import apps so their init() registers routes with the app registry.
	// Add your own apps here.
	_ "smallgo/server/medcare"
	_ "smallgo/server/qrcode"
)

func main() {
	// Local .env files are a development convenience only: values already
	// supplied by Docker, fnOS or the process environment always take priority.
	config.LoadDotEnv(".env", "server/.env")
	config.Parse()
	server.Start()
}
