package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"syscall"
	"time"

	"github.com/spf13/viper"
)

const (
	srvcSQLURL              = "/sql/"
	srvcLoginURL            = "/login"
	srvcLogoutURL           = "/logout"
	srvcDocURL              = "/doc"
	srvcExpectedContentType = "application/x-www-form-urlencoded"
	srvcOutputContentType   = "application/json;charset=UTF-8"
	srvcOutputHTMLType      = "text/html; charset=utf-8"
	srvcCfgPath             = "."
	srvcCfgName             = "config"
	srvcAuthCookieName      = "Authorization"
)

type pgmusql struct {
	httpsrv       *http.Server
	startTime     time.Time
	cfg           *viper.Viper
	queries       map[string]*query
	db            *database
	parser        *sqlParser
	timeout       time.Duration
	socketFile    *os.File
	listener      net.Listener
	mainContext   context.Context
	sessions      *sessions
	keepalive     bool
	loginRequired bool
	loginQuery    string
	logoutQuery   string
	cookieSession bool
	useTLS        bool
	sqlTreeView   *sqlTreeViewNode
}

// create pgmusql service
func pgmusqlNew(ctx context.Context, isChild bool, cfgFile string) (*pgmusql, error) {
	var p pgmusql
	var err error
	// create config
	if p.cfg, err = cfgNew(cfgFile); err != nil {
		return nil, err
	}

	// sort of config
	p.timeout = p.cfg.GetDuration("querytimeout")
	p.keepalive = p.cfg.GetBool("keepalive")
	p.loginRequired = p.cfg.GetBool("loginrequired")
	p.cookieSession = p.cfg.GetBool("cookiesession")
	p.useTLS = p.cfg.GetBool("usetls")
	p.mainContext = ctx

	// connect to db
	if p.db, err = dbNew(p.cfg.GetString("dburl"), p.cfg.GetBool("filteroutparams"), p.cfg.GetBool("filterinparams"), p.cfg.GetBool("mutedberrors")); err != nil {
		return nil, err
	}

	// create parser
	if p.parser, err = sqlParserNew(); err != nil {
		return nil, err
	}

	// parse files
	var queries map[string]*query
	ignorerrors := p.cfg.GetBool("ignorerrors")
	sqlroot := p.cfg.GetString("sqlroot")
	if queries, err = p.parser.loadSQLFiles(sqlroot, ignorerrors); err != nil {
		return nil, err
	}
	p.queries = queries

	// test queries
	if p.cfg.GetBool("autotest") {
		workers := p.cfg.GetUint("testworkers")
		if workers <= 0 {
			workers = 1
		}
		if err := p.queryTestRun(workers, ignorerrors); err != nil {
			return nil, err
		}
	}

	// create server
	mu := http.NewServeMux()
	mu.HandleFunc(srvcSQLURL, p.sqlHandler)

	// create sessions list
	if p.loginRequired {
		p.sessions = sessionsNew(p.mainContext, p.cfg.GetDuration("sessionlifetime"))
		p.loginQuery = p.cfg.GetString("loginquery")
		p.logoutQuery = p.cfg.GetString("logoutquery")
		mu.HandleFunc(srvcLoginURL, p.loginHandler)
		mu.HandleFunc(srvcLogoutURL, p.logoutHandler)
	}

	if p.cfg.GetBool("docenable") {
		mu.HandleFunc(srvcDocURL, p.docHandler)
		// delete this

		mu.Handle("/html/", http.StripPrefix("/html", http.FileServer(http.Dir("./html"))))

		// end
		p.sqlTreeView = newSQLTreeView()
		for _, query := range p.queries {
			p.sqlTreeView.add(srvcDocURL, query)
		}
		p.sqlTreeView.sort()

	}

	p.httpsrv = &http.Server{
		Addr:    p.cfg.GetString("address"),
		Handler: mu,
	}

	// init socket file and listener or inherit its
	if isChild {
		p.socketFile = os.NewFile(uintptr(3), "socketFile")
		if p.listener, err = net.FileListener(p.socketFile); err != nil {
			return nil, err
		}
		if err = syscall.Kill(syscall.Getppid(), syscall.SIGINT); err != nil {
			log.Fatalln(err)
		}
	} else {
		if p.listener, err = net.Listen("tcp", p.httpsrv.Addr); err != nil {
			return nil, err
		}
		if p.socketFile, err = p.listener.(*net.TCPListener).File(); err != nil {
			return nil, err
		}
	}

	return &p, nil
}

// server start routine
func (srvc *pgmusql) start() error {
	srvc.startTime = time.Now()
	log.Println("Start server")

	if srvc.useTLS {
		return srvc.httpsrv.ServeTLS(srvc.listener, srvc.cfg.GetString("certfile"), srvc.cfg.GetString("keyfile"))
	}

	return srvc.httpsrv.Serve(srvc.listener)
}

// server stop routine
func (srvc *pgmusql) stop() {
	log.Println("Server stops")
	ctx := srvc.mainContext
	timeout := srv.cfg.GetDuration("querytimeout")
	if timeout > 0 {
		var cancelFn context.CancelFunc
		ctx, cancelFn = context.WithTimeout(srvc.mainContext, timeout)
		defer cancelFn()
	}

	defer srvc.db.close()
	if err := srvc.httpsrv.Shutdown(ctx); err != nil {
		log.Println(err)
	}

	if srvc.loginRequired {
		srvc.sessions.gcStop()
	}

	log.Println("Server stop complete")
}

// server terminate routine
func (srvc *pgmusql) terminate() {
	log.Println("Server terminations")
	if err := srvc.httpsrv.Close(); err != nil {
		log.Println(err)
	}

	if srvc.loginRequired {
		srvc.sessions.gcStop()
	}

	log.Println("Sever termination complete")
}

// execute query function
var errQueryTimeout = errors.New("Query timeout")
var errQueryContexDone = errors.New("Context done")
var errQueryNotFound = errors.New("Query not found")

func (srvc *pgmusql) runQuery(ctx context.Context, queryname string, params url.Values, limit int) ([]byte, int, error) {
	// search query for address
	query, found := srvc.queries[queryname]
	if !found {
		return nil, 0, errQueryNotFound
	}
	// error during autotest
	if query.err != nil {
		return nil, 0, query.err
	}

	// execute query
	cancelCtx, ctxCancelFnc := context.WithCancel(ctx)
	defer ctxCancelFnc()

	timeout := time.After(srvc.timeout)
	if query.timeout != nil {
		timeout = time.After(*query.timeout)
	}

	// handle result
	select {
	// cancel
	case <-cancelCtx.Done():
		return nil, 0, errQueryContexDone
	// timeout
	case <-timeout:
		return nil, 0, errQueryTimeout
	// ok
	case response := <-srvc.db.queryChannel(cancelCtx, *query, params, limit):
		if response.err != nil {
			return nil, 0, response.err
		}
		return response.res, response.total, nil
	}
}

// parse session key
var errNoSession = errors.New("No session key")

func (srvc *pgmusql) getAuthkey(req *http.Request) (string, error) {
	var authkey string
	if srvc.cookieSession {
		cookie, err := req.Cookie(srvcAuthCookieName)
		if err != nil {
			return "", err
		}
		authkey = cookie.Value
	} else {
		authkey = req.Header.Get("Authorization")
	}

	if authkey == "" {
		return "", errNoSession
	}

	return authkey, nil
}

// check request and prepare form data
var errContentType = fmt.Errorf("Only %s content type allowed", srvcExpectedContentType)

func (srvc *pgmusql) checkRequest(req *http.Request) (int, error) {
	// check content type
	if req.Header.Get("Content-Type") != srvcExpectedContentType {
		return http.StatusBadRequest, errContentType
	}

	// parse form
	if err := req.ParseForm(); err != nil {
		return http.StatusInternalServerError, err
	}

	return 200, nil
}

// write success result
func (srvc *pgmusql) sqlWriteSuccess(rw http.ResponseWriter, result []byte) {
	rw.Header().Set("Content-Type", srvcOutputContentType)
	if srvc.keepalive {
		rw.Header().Set("Connection", "Keep-Alive")
	}

	if _, err := rw.Write(result); err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
}

// execution query handler
func (srvc *pgmusql) sqlHandler(rw http.ResponseWriter, req *http.Request) {
	// check request
	if code, err := srvc.checkRequest(req); err != nil {
		http.Error(rw, err.Error(), code)
		return
	}

	queryname := req.URL.Path[len(srvcSQLURL)-1:]

	// check session and login query
	var err error
	if srvc.loginRequired {
		if queryname == srvc.loginQuery || queryname == srvc.logoutQuery {
			http.Error(rw, "Calling this query directly is prohibited.", http.StatusForbidden)
			return
		}

		var authkey string
		if authkey, err = srvc.getAuthkey(req); err != nil {
			http.Error(rw, err.Error(), http.StatusUnauthorized)
			return
		}

		if !srvc.sessions.check(authkey) {
			http.Error(rw, "Session key is invalid", http.StatusUnauthorized)
			return
		}
	}

	// run query
	var res []byte
	if res, _, err = srvc.runQuery(req.Context(), queryname, req.Form, 0); err != nil {
		switch err {
		case errQueryNotFound:
			http.NotFound(rw, req)
		case errQueryTimeout:
			http.Error(rw, "Query timeout", http.StatusGatewayTimeout)
		default:
			http.Error(rw, err.Error(), http.StatusInternalServerError)
		}

		return
	}

	// Success result
	srvc.sqlWriteSuccess(rw, res)
}

// login handler
var errLoginNoData = errors.New("No data")

func (srvc *pgmusql) loginHandler(rw http.ResponseWriter, req *http.Request) {
	// check request
	if code, err := srvc.checkRequest(req); err != nil {
		http.Error(rw, err.Error(), code)
		return
	}

	// run query
	var res []byte
	var total int
	var err error
	if res, total, err = srvc.runQuery(req.Context(), srvc.loginQuery, req.Form, 0); err != nil || total == 0 {
		if err == nil {
			err = errLoginNoData
		}
		http.Error(rw, err.Error(), http.StatusForbidden)
		return
	}

	// create session and return data
	var session string
	var expire time.Time
	if session, expire, err = srvc.sessions.new(); err != nil {
		http.Error(rw, err.Error(), http.StatusForbidden)
		return
	}

	if srvc.cookieSession {
		var cookie http.Cookie
		cookie.Name = srvcAuthCookieName
		cookie.Value = session
		cookie.HttpOnly = true
		if srvc.useTLS {
			cookie.Secure = true
		}
		cookie.Expires = expire

		http.SetCookie(rw, &cookie)
	} else {
		rw.Header().Set("Authorization", session)
	}

	// Success result
	srvc.sqlWriteSuccess(rw, res)
}

// logout handler
func (srvc *pgmusql) logoutHandler(rw http.ResponseWriter, req *http.Request) {
	// check request
	if code, err := srvc.checkRequest(req); err != nil {
		http.Error(rw, err.Error(), code)
		return
	}

	// get session key
	var authkey string
	var err error
	if authkey, err = srvc.getAuthkey(req); err != nil {
		http.Error(rw, err.Error(), http.StatusUnauthorized)
		return
	}

	var res []byte
	if srvc.logoutQuery != "" {
		// run query

		var total int
		if res, total, err = srvc.runQuery(req.Context(), srvc.logoutQuery, req.Form, 0); err != nil || total == 0 {
			if err == nil {
				err = errLoginNoData
			}
			http.Error(rw, err.Error(), http.StatusForbidden)
			return
		}
	}

	// delete session key
	srvc.sessions.logout(authkey)

	// Success result
	srvc.sqlWriteSuccess(rw, res)
}
