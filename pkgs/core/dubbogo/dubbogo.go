package dubbogo

import (
	"context"
	"fmt"
	"github.com/dubbo/dubbo-go/jsonrpc"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path"
	"strconv"
	"syscall"
	"time"
)

import (
	"github.com/AlexStocks/goext/log"
	"github.com/AlexStocks/goext/net"
	log "github.com/AlexStocks/log4go"
	jerrors "github.com/juju/errors"
)

import (
	"github.com/dubbo/dubbo-go/public"
	"github.com/dubbo/dubbo-go/registry"
)

var (
	survivalTimeout int = 10e9
	clientRegistry  *registry.ZkConsumerRegistry
)

const (
	APP_CONF_FILE     = "APP_CONF_FILE"
	APP_LOG_CONF_FILE = "APP_LOG_CONF_FILE"
)

var (
	clientConfig *ClientConfig
)

type Clienter interface {
	Prepare() *DubboClient
	ClientFunc(data interface{}) interface{}
}

type (
	// Client holds supported types by the multiconfig package
	ClientConfig struct {
		// pprof
		Pprof_Enabled bool `default:"false" yaml:"pprof_enabled" json:"pprof_enabled,omitempty"`
		Pprof_Port    int  `default:"10086"  yaml:"pprof_port" json:"pprof_port,omitempty"`

		// client
		Connect_Timeout string `default:"100ms"  yaml:"connect_timeout" json:"connect_timeout,omitempty"`
		connectTimeout  time.Duration

		Request_Timeout string `yaml:"request_timeout" default:"5s" json:"request_timeout,omitempty"` // 500ms, 1m
		requestTimeout  time.Duration

		// codec & selector & transport & registry
		Selector     string `default:"cache"  yaml:"selector" json:"selector,omitempty"`
		Selector_TTL string `default:"10m"  yaml:"selector_ttl" json:"selector_ttl,omitempty"`
		Registry     string `default:"zookeeper"  yaml:"registry" json:"registry,omitempty"`
		// application
		Application_Config registry.ApplicationConfig `yaml:"application_config" json:"application_config,omitempty"`
		Registry_Config    registry.RegistryConfig    `yaml:"registry_config" json:"registry_config,omitempty"`
		// 一个客户端只允许使用一个service的其中一个group和其中一个version
		Service_List []registry.ServiceConfig `yaml:"service_list" json:"service_list,omitempty"`
	}

	DubboClient struct {
		ctl         *jsonrpc.HTTPClient
		ctx         context.Context
		ServiceName string
		Method      string
		serviceUrl  *registry.ServiceURL
		req         jsonrpc.Request
		Args        interface{}
	}
)

func InitDubbo() {
	err := initClientConfig()
	if err != nil {
		log.Error("initClientConfig() = error{%#v}", err)
		panic(err)
	}
	initProfiling()
	initClient()
}

func DubboClientRun() {
	initSignal()
}

func initClient() {
	var (
		err       error
		codecType public.CodecType
	)

	if clientConfig == nil {
		panic(fmt.Sprintf("clientConfig is nil"))
		return
	}

	// registry
	clientRegistry, err = registry.NewZkConsumerRegistry(
		registry.ApplicationConf(clientConfig.Application_Config),
		registry.RegistryConf(clientConfig.Registry_Config),
		registry.BalanceMode(registry.SM_RoundRobin),
		registry.ServiceTTL(300e9),
	)
	if err != nil {
		panic(fmt.Sprintf("fail to init registry.Registy, err:%s", jerrors.ErrorStack(err)))
		return
	}
	for _, service := range clientConfig.Service_List {
		err = clientRegistry.Register(service)
		if err != nil {
			panic(fmt.Sprintf("registry.Register(service{%#v}) = error{%v}", service, jerrors.ErrorStack(err)))
			return
		}
	}

	// consumer
	clientConfig.requestTimeout, err = time.ParseDuration(clientConfig.Request_Timeout)
	if err != nil {
		panic(fmt.Sprintf("time.ParseDuration(Request_Timeout{%#v}) = error{%v}",
			clientConfig.Request_Timeout, err))
		return
	}
	clientConfig.connectTimeout, err = time.ParseDuration(clientConfig.Connect_Timeout)
	if err != nil {
		panic(fmt.Sprintf("time.ParseDuration(Connect_Timeout{%#v}) = error{%v}",
			clientConfig.Connect_Timeout, err))
		return
	}

	for idx := range clientConfig.Service_List {
		codecType = public.GetCodecType(clientConfig.Service_List[idx].Protocol)
		if codecType == public.CODECTYPE_UNKNOWN {
			panic(fmt.Sprintf("unknown protocol %s", clientConfig.Service_List[idx].Protocol))
		}
	}
}

func uninitClient() {
	log.Close()
}

func initProfiling() {
	if !clientConfig.Pprof_Enabled {
		return
	}
	const (
		PprofPath = "/debug/pprof/"
	)
	var (
		err  error
		ip   string
		addr string
	)

	ip, err = gxnet.GetLocalIP()
	if err != nil {
		panic("cat not get local ip!")
	}
	addr = ip + ":" + strconv.Itoa(clientConfig.Pprof_Port)
	log.Info("App Profiling startup on address{%v}", addr+PprofPath)
	fmt.Println(addr + PprofPath)
	go func() {
		log.Info(http.ListenAndServe(addr, nil))
	}()
}

func initSignal() {
	signals := make(chan os.Signal, 1)
	// It is not possible to block SIGKILL or syscall.SIGSTOP
	signal.Notify(signals, os.Interrupt, os.Kill, syscall.SIGHUP,
		syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT)
	for {
		sig := <-signals
		log.Info("get signal %s", sig.String())
		switch sig {
		case syscall.SIGHUP:
		// reload()
		default:
			go time.AfterFunc(time.Duration(survivalTimeout)*time.Second, func() {
				log.Warn("app exit now by force...")
				os.Exit(1)
			})

			// 要么fastFailTimeout时间内执行完毕下面的逻辑然后程序退出，要么执行上面的超时函数程序强行退出
			uninitClient()
			fmt.Println("app exit now...")
			return
		}
	}
}

func initClientConfig() error {
	var (
		confFile string
	)

	// configure
	confFile = os.Getenv(APP_CONF_FILE)
	if confFile == "" {
		panic(fmt.Sprintf("application configure file name is nil"))
		return nil // I know it is of no usage. Just Err Protection.
	}
	if path.Ext(confFile) != ".yml" {
		panic(fmt.Sprintf("application configure file name{%v} suffix must be .yml", confFile))
		return nil
	}
	clientConfig = new(ClientConfig)

	confFileStream, err := ioutil.ReadFile(confFile)
	if err != nil {
		panic(fmt.Sprintf("ioutil.ReadFile(file:%s) = error:%s", confFile, jerrors.ErrorStack(err)))
		return nil
	}
	err = yaml.Unmarshal(confFileStream, clientConfig)
	if err != nil {
		panic(fmt.Sprintf("yaml.Unmarshal() = error:%s", jerrors.ErrorStack(err)))
		return nil
	}
	if clientConfig.Registry_Config.Timeout, err = time.ParseDuration(clientConfig.Registry_Config.TimeoutStr); err != nil {
		panic(fmt.Sprintf("time.ParseDuration(Registry_Config.Timeout:%#v) = error:%s", clientConfig.Registry_Config.TimeoutStr, err))
		return nil
	}

	gxlog.CInfo("config{%#v}\n", clientConfig)

	// log
	confFile = os.Getenv(APP_LOG_CONF_FILE)
	if confFile == "" {
		panic(fmt.Sprintf("log configure file name is nil"))
		return nil
	}
	if path.Ext(confFile) != ".xml" {
		panic(fmt.Sprintf("log configure file name{%v} suffix must be .xml", confFile))
		return nil
	}
	log.LoadConfiguration(confFile)

	return nil
}

func (dCli *DubboClient) ClientFunc(data interface{}) error {
	if dCli == nil {
		return fmt.Errorf("dCli is nil,prepare return error")
	}
	// Call service
	if err := dCli.ctl.Call(dCli.ctx, *dCli.serviceUrl, dCli.req, data); err != nil {
		log.Error("client.Call() return error:%+v", jerrors.ErrorStack(err))
		return err
	}

	log.Info("response result:%s", data)
	return nil
}

func (dCli *DubboClient) Prepare() *DubboClient {
	var (
		err        error
		serviceIdx int
	)

	dCli.ctl = jsonrpc.NewHTTPClient(
		&jsonrpc.HTTPOptions{
			HandshakeTimeout: clientConfig.connectTimeout,
			HTTPTimeout:      clientConfig.requestTimeout,
		},
	)

	serviceIdx = -1
	for i := range clientConfig.Service_List {
		if clientConfig.Service_List[i].Service == dCli.ServiceName &&
			clientConfig.Service_List[i].Protocol == public.CODECTYPE_JSONRPC.String() {
			serviceIdx = i
			break
		}
	}
	if serviceIdx == -1 {
		log.Error(fmt.Sprintf("can not find service in config service list:%#v", clientConfig.Service_List))
		return nil
	}

	conf := registry.ServiceConfig{
		Group:    clientConfig.Service_List[serviceIdx].Group,
		Protocol: public.CodecType(public.CODECTYPE_JSONRPC).String(),
		Version:  clientConfig.Service_List[serviceIdx].Version,
		Service:  clientConfig.Service_List[serviceIdx].Service,
	}
	dCli.req = dCli.ctl.NewRequest(conf, dCli.Method, dCli.Args)

	dCli.serviceUrl, err = clientRegistry.Filter(dCli.req.ServiceConfig(), 1)
	if err != nil {
		log.Error("registry.Filter(conf:%#v) = error:%s", dCli.req.ServiceConfig(), jerrors.ErrorStack(err))
		return nil
	}
	log.Debug("got serviceURL: %s", dCli.serviceUrl)
	dCli.ctx = context.WithValue(context.Background(), public.DUBBOGO_CTX_KEY, map[string]string{
		"X-Proxy-Id": "dubbogo",
		"X-Services": dCli.ServiceName,
		"X-Method":   dCli.Method,
	})
	return dCli
}
