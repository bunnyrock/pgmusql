package main

import (
	"encoding/json"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"time"
)

type docParam struct {
	Name        string
	Description string
	Warning     string
}

type sqlDescription struct {
	Name         string
	Description  string
	In           []docParam
	Out          []docParam
	LoadTime     string
	Timeout      string
	ParseWarn    string
	TestPass     string
	TestParams   []docParam
	TestDuration string
	Err          string
	TestResult   string
	HasWarn      bool
	HasErr       bool
}

func (d *sqlDescription) parse(q *query) {
	// name
	d.Name = q.name

	d.HasWarn = false

	// description
	if d.Description = q.description; d.Description == "" {
		d.HasWarn = true
	}

	// in params
	d.In = make([]docParam, 0)

	for _, param := range q.in {
		warn := ""

		if find, _ := q.params.find(param.key); !find {
			warn = "Declared and not used"
			d.HasWarn = true
		}

		d.In = append(d.In, docParam{param.key, param.value, warn})
	}

	for _, param := range q.params {
		if find, _ := q.in.find(param); !find {
			d.In = append(d.In, docParam{param, "", "Used but not declared"})
			d.HasWarn = true
		}
	}

	// out params
	d.Out = make([]docParam, 0)

	testResult := make([]map[string]interface{}, 0)
	hasTest := false
	if q.testreport != nil && q.err == nil {
		if json.Unmarshal(q.testreport.testResult, &testResult) == nil && len(testResult) > 0 {
			hasTest = true
		}
	}

	for _, param := range q.out {
		warn := ""
		if hasTest {
			if _, ok := testResult[0][param.key]; !ok {
				warn = "Field not found in test request"
				d.HasWarn = true
			}
		}

		d.Out = append(d.Out, docParam{param.key, param.value, warn})
	}

	if hasTest {
		for column := range testResult[0] {
			if find, _ := q.out.find(column); !find {
				d.Out = append(d.Out, docParam{column, "", "Field found in test request but not described"})
				d.HasWarn = true
			}
		}
	}

	// load time
	d.LoadTime = q.loadtime.Format("2006-01-02 15:04:05")

	// timeout
	d.Timeout = "Default"
	if q.timeout != nil {
		d.Timeout = q.timeout.String()
	}

	// parse warn
	if d.ParseWarn = q.parsewarn; d.ParseWarn != "" {
		d.HasWarn = true
	}

	// test pass
	d.TestPass = q.testpass.String()

	// test params
	d.TestParams = make([]docParam, 0)

	for _, param := range q.testparams {
		d.TestParams = append(d.TestParams, docParam{param.key, param.value, ""})
	}

	// test duration
	if q.testreport != nil {
		d.TestDuration = q.testreport.endTime.Sub(q.testreport.startTime).Round(time.Millisecond).String()

		// test result
		if jres, err := json.MarshalIndent(testResult, "", "    "); err != nil {
			d.TestResult = err.Error()
			d.HasWarn = true
		} else {
			d.TestResult = string(jres)
		}
	}

	// error
	if q.err != nil {
		d.HasErr = true
		d.Err = q.err.Error()
	}
}

type sqlTreeViewNode struct {
	IsFile      bool
	IsRoot      bool
	Name        string
	Path        string
	Description sqlDescription
	Childs      []*sqlTreeViewNode
}

func newSQLTreeView() *sqlTreeViewNode {
	resut := sqlTreeViewNode{
		IsFile: false,
		IsRoot: true,
		Name:   "sql",
		Path:   "/",
		Childs: make([]*sqlTreeViewNode, 0),
	}
	return &resut
}

func (node *sqlTreeViewNode) hasChild(name string) (bool, *sqlTreeViewNode) {
	for _, c := range node.Childs {
		if c.Name == name {
			return true, c
		}
	}
	return false, nil
}

func (node *sqlTreeViewNode) add(rootURL string, query *query) {
	curDir := node
	dirNames := strings.Split(query.name, "/")
	var curPath string

	for i, name := range dirNames {
		if name == "" {
			continue
		}

		curPath += "/" + name

		if yes, dir := curDir.hasChild(name); yes {
			curDir = dir
			continue
		}

		// cause last element always is file
		isFile := false
		if i == (len(dirNames) - 1) {
			isFile = true
		}

		newNode := sqlTreeViewNode{
			IsFile: isFile,
			Name:   name,
			Path:   curPath,
			Childs: make([]*sqlTreeViewNode, 0),
		}

		if isFile {
			newNode.Description.parse(query)
		}

		curDir.Childs = append(curDir.Childs, &newNode)
		curDir = &newNode
	}
}

func (node *sqlTreeViewNode) sort() {
	sort.Slice(node.Childs, func(i, j int) bool {
		nodi := node.Childs[i]
		nodj := node.Childs[j]

		return (!nodi.IsFile && nodj.IsFile) || (nodi.Name < nodj.Name && nodi.IsFile == nodj.IsFile)
	})

	for _, c := range node.Childs {
		c.sort()
	}
}

func (srvc *pgmusql) docHandler(rw http.ResponseWriter, req *http.Request) {
	var docTmplt *template.Template
	var err error

	if docTmplt, err = template.ParseGlob("html/*.gohtml"); err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
	}

	rw.Header().Set("Content-Type", srvcOutputHTMLType)

	if err := docTmplt.ExecuteTemplate(rw, "Main", srvc.sqlTreeView); err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
	}
}
