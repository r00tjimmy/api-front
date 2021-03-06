package proxy

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"github.com/googollee/go-socket.io"
	"github.com/hidu/goutils"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const userCookieName = "_apiman"

// APIProxyVersion current server version
var APIProxyVersion string

func init() {
	APIProxyVersion = strings.TrimSpace(Assest.GetContent("/res/version"))
}

type webAdmin struct {
	apiServer *APIServer
	wsServer  *socketio.Server
	wsSocket  socketio.Socket
	userConf  *usersConf
}

func newWebAdmin(server *APIServer) *webAdmin {
	ser := &webAdmin{
		apiServer: server,
	}
	ser.wsInit()
	ser.userConf = loadUsers(filepath.Join(server.rootConfDir(), "users"))
	return ser
}
func (web *webAdmin) wsInit() {
	server, err := socketio.NewServer(nil)
	if err != nil {
		log.Fatalln("init ws server failed:", err.Error())
	}
	web.wsServer = server

	server.On("connection", func(so socketio.Socket) {
		so.Emit("hello", "hello,now:"+time.Now().String())
		so.On("disconnection", func() {
			log.Println("on disconnect")
		})
		web.wsSocket = so
		so.Join("api_pv")
		so.On("http_analysis", func(name string) {
			api := web.apiServer.getAPIByName(name)
			if api != nil {
				err := so.Join(api.roomName())
				log.Println("join_room", api.roomName(), err)
			}
		})
	})
	server.On("error", func(so socketio.Socket) {
		log.Println("ws error:", err)
	})
	web.broadAPIPvs()
}
func (web *webAdmin) broadAPIPvs() {
	pvs := make(map[string]uint64)

	utils.SetInterval(func() {
		var pv uint64
		for name, api := range web.apiServer.Apis {
			if _, has := pvs[name]; !has {
				pvs[name] = 0
			}
			pv = api.GetPv()
			if pvs[name] == pv {
				continue
			}
			pvs[name] = pv
			data := make(map[string]interface{})
			data["name"] = name
			data["pv"] = pv
			web.wsServer.BroadcastTo("api_pv", "api_pv", data)
		}
	}, 1)
}

func (web *webAdmin) broadcastAPI(api *apiStruct, broadType string, reqData *BroadCastData) {
	roomName := api.roomName()
	log.Println("broad:", roomName, broadType)
	web.wsServer.BroadcastTo(roomName, broadType, reqData)
}

func (web *webAdmin) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	web.serveHTTP(rw, req)
	return
	if !strings.Contains(req.Header.Get("Accept-Encoding"), "gzip") || strings.HasPrefix(req.URL.Path, "/_socket.io/") {
		web.serveHTTP(rw, req)
		return
	}
	rw.Header().Set("Content-Encoding", "gzip")
	gz := gzip.NewWriter(rw)
	defer gz.Close()
	gzr := gzipResponseWriter{Writer: gz, ResponseWriter: rw}
	web.serveHTTP(gzr, req)
}

func (web *webAdmin) serveHTTP(rw http.ResponseWriter, req *http.Request) {
	if strings.HasPrefix(req.URL.Path, "/_res/") {
		req.URL.Path = "/res/" + req.URL.Path[5:]
		Assest.HTTPHandler("/").ServeHTTP(rw, req)
		return
	}
	if strings.HasPrefix(req.URL.Path, "/_socket.io/") {
		web.wsServer.ServeHTTP(rw, req)
		return
	}

	wr := &webReq{
		rw:     rw,
		req:    req,
		web:    web,
		values: make(map[string]interface{}),
	}
	wr.execute()
}

type webReq struct {
	rw     http.ResponseWriter
	req    *http.Request
	web    *webAdmin
	values map[string]interface{}
	user   *user
}

func (wr *webReq) execute() {
	wr.values["Title"] = "Index"
	wr.values["version"] = APIProxyVersion
	wr.values["base_url"] = "http://" + wr.req.Host
	hostInfo := strings.Split(wr.req.Host, ":")
	if hostInfo[1] == "" {
		hostInfo[1] = "80"
	}
	wr.values["req_host"] = wr.req.Host
	wr.values["host_name"] = hostInfo[0]
	port, _ := strconv.ParseInt(hostInfo[1], 10, 64)
	wr.values["host_port"] = int(port)
	wr.values["conf"] = wr.web.apiServer.ServerConf
	wr.getUser()

	wr.values["isLogin"] = wr.user != nil
	if wr.user != nil {
		wr.values["uname"] = wr.user.Name
	}

	if wr.req.Method == "POST" && wr.req.URL.Path != "/_login" && wr.user == nil {
		wr.alert("login required")
		return
	}

	switch wr.req.URL.Path {
	case "/_api":
		wr.values["Title"] = "Edit"
		wr.apiEdit()
		return
	case "/_apis":
		wr.values["Title"] = "List"
		wr.apiList()
		return
	case "/_pref":
		wr.apiPref()
		return
	case "/_apipv":
		wr.apiPv()
		return
	case "/_login":
		wr.values["Title"] = "Login"
		wr.login()
		return
	case "/_logout":
		wr.logout()
		return
	case "/_analysis":
		wr.values["Title"] = "Analysis"
		wr.apiAnalysis()
		return
	}

	userIndexHTMLPath := wr.web.apiServer.rootConfDir() + "index.html"
	wr.values["userIndex"] = loadFile(userIndexHTMLPath)
	wr.render("index.html", true)
}

func (wr *webReq) getUser() {
	cookie, err := wr.req.Cookie(userCookieName)
	if err != nil {
		return
	}
	info := strings.SplitN(cookie.Value, ":", 2)
	if len(info) != 2 || len(info[1]) != 32 {
		return
	}
	user := wr.web.userConf.getUser(info[0])
	if user != nil && user.pswEnc() == info[1] {
		wr.user = user
	}
}

func (wr *webReq) apiList() {
	wr.values["apis"] = wr.web.apiServer.Apis
	wr.render("list.html", true)
}

func (wr *webReq) logout() {
	cookie := &http.Cookie{Name: userCookieName, Value: "", Path: "/"}
	http.SetCookie(wr.rw, cookie)
	http.Redirect(wr.rw, wr.req, "/_index", 302)
}

func (wr *webReq) login() {
	if wr.req.Method == "POST" {
		name := wr.req.PostFormValue("name")
		psw := wr.req.PostFormValue("psw")
		user := wr.web.userConf.checkUser(name, psw)
		if user == nil {
			log.Println("[warning]login failed;user:", name)
			wr.alert("login failed")
			return
		}
		cookie := &http.Cookie{
			Name:    userCookieName,
			Value:   fmt.Sprintf("%s:%s", name, user.pswEnc()),
			Path:    "/",
			Expires: time.Now().Add(24 * 30 * time.Hour),
		}
		http.SetCookie(wr.rw, cookie)
		wr.rw.Write([]byte("<script>parent.location.href='/_index'</script>"))
	} else {
		wr.render("login.html", true)
	}
}

func (wr *webReq) apiPref() {
	apiName := strings.TrimSpace(wr.req.FormValue("name"))
	prefHost := strings.TrimSpace(wr.req.FormValue("host"))

	if apiName == "" {
		wr.json(400, "param empty", nil)
		return
	}

	cookieName := apiCookieName(apiName)

	if prefHost != "" {
		api := wr.web.apiServer.getAPIByName(apiName)
		if api == nil {
			wr.json(400, "api not exists", nil)
			return
		}
	}

	cookie := &http.Cookie{Name: cookieName, Value: prefHost, Path: "/"}
	if prefHost != "" {
		cookie.Expires = time.Now().AddDate(1, 0, 0)
		cookie.MaxAge = 86400 * 365
	} else {
		cookie.MaxAge = -1
	}

	http.SetCookie(wr.rw, cookie)

	wr.json(0, "success", prefHost)
}

func (wr *webReq) apiPv() {
	apiName := strings.TrimSpace(wr.req.FormValue("name"))
	if apiName == "" {
		wr.json(400, "param empty", nil)
		return
	}
	api := wr.web.apiServer.getAPIByName(apiName)
	if api == nil {
		wr.json(400, "api not exists", nil)
		return
	}
	wr.json(0, "suc", api.GetPv())
}
func (wr *webReq) apiAnalysis() {
	apiName := strings.TrimSpace(wr.req.FormValue("name"))
	wr.values["Title"] = "Analysis-" + apiName
	if apiName == "" {
		wr.values["error"] = "param empty"
		wr.render("error.html", true)
		return
	}
	api := wr.web.apiServer.getAPIByName(apiName)
	if api == nil {
		wr.values["error"] = "api not exists!  <a href='/_api'>add new</a>"
		wr.render("error.html", true)
		return
	}
	wr.values["api"] = api
	wr.render("analysis.html", true)
}

func (wr *webReq) alert(msg string) {
	wr.rw.Write([]byte(fmt.Sprintf(`<script>alert("%s")</script>`, msg)))
}
func (wr *webReq) alertAndGo(msg string, urlstr string) {
	wr.rw.Write([]byte(fmt.Sprintf(`<script>alert("%s");top.location.href="%s";</script>`, msg, urlstr)))
}

// JSONResult json result when ajax call
type JSONResult struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

func (wr *webReq) json(code int, msg string, data interface{}) {
	ret := &JSONResult{code, msg, data}
	bs, _ := json.Marshal(ret)
	wr.rw.Header().Set("Content-Type", "application/json;charset=utf-8")
	wr.rw.Write(bs)
}

func (wr *webReq) render(tplName string, layout bool) {
	html := renderHTML(tplName, wr.values, true)
	wr.rw.Header().Set("Content-Type", "text/html;charset=utf-8")
	wr.rw.Write([]byte(html))
}

var webTmpName = "tpl_api_proxy"

func (wr *webReq) apiEdit() {
	req := wr.req
	name := req.FormValue("name")
	if req.Method != "POST" {
		var api *apiStruct
		if name != "" {
			apiOld := wr.web.apiServer.getAPIByName(name)
			if apiOld == nil {
				wr.values["error"] = "api not exists!  <a href='/_api'>add new</a>"
				wr.render("error.html", true)
				return
			}
			api = apiOld.clone()
		} else {
			api = newAPI(wr.web.apiServer, "")
		}
		hostsTpl := newHosts()
		hostsTpl.addNewHost(newHost(webTmpName, "http://127.0.0.1/", false))

		citem, _ := newCallerItem("")
		api.Caller.addNewCallerItem(citem)

		wr.values["api"] = &api
		wr.values["HostsTpl"] = hostsTpl
		wr.values["api_url"] = "http://" + req.Host + api.Path

		wr.values["userCanEdit"] = api.userCanEdit(wr.user)

		prefCookie, err := wr.req.Cookie(api.cookieName())
		cookiePref := ""
		if err == nil {
			cookiePref = prefCookie.Value
		}
		wr.values["cookiePref"] = strings.Split(cookiePref, ",")
		wr.values["cookiePrefStr"] = cookiePref

		wr.render("api.html", true)
		return
	}

	do := req.FormValue("do")
	switch do {
	case "base":
		wr.apiBaseSave()
	case "caller":
		wr.apiCallerSave()
	case "rename":
		wr.apiRename()
	default:
		wr.alert("unknow")
	}
}

func (wr *webReq) apiRename() {
	req := wr.req
	origName := req.FormValue("orig_name")
	newName := req.FormValue("new_name")

	if origName == newName {
		wr.json(304, "now change", nil)
		return
	}
	if !apiNameReg.MatchString(newName) {
		wr.json(400, "name wrong", nil)
		return
	}

	origApi := wr.web.apiServer.getAPIByName(origName)
	if origApi == nil {
		wr.json(404, "api not found", nil)
		return
	}

	newApi := wr.web.apiServer.getAPIByName(newName)
	if newApi != nil {
		wr.json(404, newName+" aready exists!", nil)
		return
	}

	if err := origApi.reName(newName); err != nil {
		wr.json(500, "rename failed", nil)
		return
	}
	wr.web.apiServer.unRegisterAPI(origName)
	wr.web.apiServer.loadAPI(newName)
	wr.json(0, "success", newName)
}

func (wr *webReq) apiBaseSave() {
	req := wr.req

	mod := req.FormValue("mod")

	if mod == "new" && !wr.web.apiServer.hasUser(wr.user.Name) {
		wr.alert("没有权限!")
		return
	}

	timeout, err := strconv.ParseInt(req.FormValue("timeout"), 10, 64)
	if err != nil {
		wr.alert("超时时间错误,不是int")
		return
	}
	apiName := req.FormValue("api_name")

	//绑定路径
	apiPath := URLPathClean(req.FormValue("path"))

	if !apiNameReg.MatchString(apiName) {
		wr.alert(fmt.Sprintf(`模块名称(%s)不满足规则：^[\w-]+$`, apiName))
		return
	}

	api := wr.web.apiServer.getAPIByName(apiName)
	if api != nil && mod == "new" {
		wr.alert(fmt.Sprintf(`模块(%s)已经存在`, apiName))
		return
	}

	if api != nil && !api.userCanEdit(wr.user) {
		wr.alert("没有权限")
		return
	}

	//按照路径查找得到的api
	apiByPath := wr.web.apiServer.getAPIByPath(apiPath)

	if apiByPath != nil {
		if api == nil || (api != nil && api.Name != apiByPath.Name) {
			wr.alert(fmt.Sprintf("绑定的路径(%s)和api(%s:%s)重复", apiPath, apiByPath.Name, apiByPath.Note))
			return
		}
	}
	if api == nil {
		api = wr.web.apiServer.newAPI(apiName)
	}

	hostNames := req.PostForm["host_name"]
	hostNameOrigs := req.PostForm["host_name_orig"]
	hostUrls := req.PostForm["host_url"]
	hostNotes := req.PostForm["host_note"]
	hostEnables := req.PostForm["host_enable"]

	if len(hostNames) != len(hostUrls) || len(hostNames) != len(hostNotes) || len(hostNames) != len(hostEnables) {
		wr.alert("保存失败：数据格式错误")
		return
	}

	tmp := make(map[string]string)
	for _, val := range hostNames {
		if _, has := tmp[val]; has {
			wr.alert("别名:" + val + "，重复了")
			return
		}
	}

	for i, name := range hostNames {
		if name == "" || name == webTmpName {
			continue
		}
		host := newHost(name, hostUrls[i], true)
		host.Note = hostNotes[i]
		host.Enable = hostEnables[i] == "1"

		//		wr.web.apiServer.
		api.Hosts.addNewHost(host)
		nameOrig := hostNameOrigs[i]
		api.hostRename(nameOrig, name)
	}
	api.hostCheckDelete(hostNames)

	if api == nil {
		api = wr.web.apiServer.newAPI(apiName)
	}

	api.Note = req.FormValue("note")
	api.TimeoutMs = int(timeout)
	api.Enable = req.FormValue("enable") == "1"
	api.Path = apiPath
	api.HostAsProxy = req.FormValue("host_as_proxy") == "1"

	err = api.save()
	if err != nil {
		wr.alert("保存失败：" + err.Error())
		return
	}
	wr.web.apiServer.loadAPI(apiName)
	wr.alertAndGo("已经保存！", "/_api?name="+apiName)
}

func (wr *webReq) apiCallerSave() {
	req := wr.req
	apiName := req.FormValue("api_name")
	api := wr.web.apiServer.getAPIByName(apiName)
	if api == nil {
		wr.alert("api模块不存在")
		return
	}
	datas := req.Form["datas[]"]
	callers := newCaller()
	for _, qs := range datas {
		qv, _ := url.ParseQuery(qs)
		item, _ := newCallerItem(qv.Get("ip"))
		item.Note = qv.Get("note")
		item.Enable = qv.Get("enable") == "1"
		if qv.Get("host_names") != "" {
			item.Pref = qv["host_names"]
		}
		if qv.Get("host_ignore") != "" {
			item.Ignore = qv["host_ignore"]

			for _, ignoreName := range item.Ignore {
				if InStringSlice(ignoreName, item.Pref) {
					wr.json(1, "配置冲突("+item.IP+")\n屏蔽:"+ignoreName, nil)
					return
				}
			}
		}
		callers.addNewCallerItem(item)
	}
	api.Caller = callers

	err := api.save()
	if err != nil {
		wr.json(1, "保存配置失败:"+err.Error(), nil)
		return
	}
	wr.web.apiServer.loadAPI(apiName)
	wr.json(0, "已经更新！", nil)
}
