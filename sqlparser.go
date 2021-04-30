package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Oh dear... A little bit of unicorn shit 🦄💩
const (
	mainExpStr = `(?P<commentary>(?s)--.*?\n|/\*.*?\*/)|` +
		`(?P<simplestrings>'(?:[^'\\]|\\.)*')|` +
		`(?P<identstrings>"(?:[^"\\]|\\.)*")|` +
		`(?P<dollarstrings>(?s)\$\w*?\$.*?\$\w*?\$)|` + // maybe buggy
		`(?P<typeconv>::\w*)|` + // just a pg type conversion fix
		`:(?P<params>[_a-zA-Z]\w*)` // FINALLY!!1!!1111!

	dirExpStr = `(?is)#(?P<directivename>\w*?):(?P<directivebody>.*?)##`

	dirParamExpStr = `(?P<key>[_a-zA-Z]\w*)\s*=\s*(?s)(?P<value>.*?)\s*;`
)

const (
	expCommentaryGrp = 1
	expParamsGrp     = 6
	expDirNameGrp    = 1
	expDirBodyGrp    = 2
	expKeyGrp        = 1
	expValueGrp      = 2
)

type sqlParser struct {
	mainExp  *regexp.Regexp
	dirExp   *regexp.Regexp
	paramExp *regexp.Regexp
}

func sqlParserNew() (*sqlParser, error) {
	var err error
	var p sqlParser

	if p.mainExp, err = regexp.Compile(mainExpStr); err != nil {
		return nil, err
	}

	if p.dirExp, err = regexp.Compile(dirExpStr); err != nil {
		return nil, err
	}

	if p.paramExp, err = regexp.Compile(dirParamExpStr); err != nil {
		return nil, err
	}

	return &p, nil
}

// parse query
func (p *sqlParser) parse(str string, res *query) {
	res.loadtime = time.Now()
	res.testpass = testPassNoError //by deafault

	// read sql input parameters and construct commentary string
	cmtstr := ""
	lastParamEnd := 0
	for _, match := range p.mainExp.FindAllStringSubmatchIndex(str, -1) {
		// concat commentary string
		cmtbgn := match[expCommentaryGrp*2]
		cmtend := match[expCommentaryGrp*2+1]

		if cmtbgn != -1 && cmtend != -1 {
			cmtstr += str[cmtbgn:cmtend] + "\n"
		}

		// read sql parameters and replace named params on positional params
		parambgn := match[expParamsGrp*2]
		paramend := match[expParamsGrp*2+1]

		if parambgn != -1 && paramend != -1 {
			paramname := strings.ToLower(str[parambgn:paramend])
			found, i := res.params.find(paramname)
			if !found {
				res.params = append(res.params, paramname)
				i = len(res.params) - 1
			}

			// param at begining ?
			if (parambgn - 1) > 0 { // -1 cause param have ":" prefix
				res.body += str[lastParamEnd : parambgn-1]
			}
			res.body += "$" + strconv.Itoa(i+1)

			lastParamEnd = paramend

		}
	}

	if lastParamEnd != len(str)-1 {
		upBound := len(str) - 1
		if upBound < 0 {
			upBound = 0
		}
		res.body += str[lastParamEnd:upBound]
	}

	// parse directives
	for _, match := range p.dirExp.FindAllStringSubmatch(cmtstr, -1) {
		dirname := strings.ToLower(match[expDirNameGrp])
		dirbody := strings.TrimSpace(match[expDirBodyGrp])

		switch dirname {
		case "description":
			res.description += dirbody + "\n"
		case "timeout":
			if timeout, err := time.ParseDuration(dirbody); err == nil {
				res.timeout = &timeout
			} else {
				res.parsewarn += fmt.Sprintln("Can't parse timeout, value is ", dirbody)
			}
		case "in":
			res.in.readIn(p.paramExp, dirbody, expKeyGrp, expValueGrp)
		case "out":
			res.out.readIn(p.paramExp, dirbody, expKeyGrp, expValueGrp)
		case "test":
			res.testparams.readIn(p.paramExp, dirbody, expKeyGrp, expValueGrp)
		case "testpass":
			// available values testpass
			var testPass queryTestPassType
			err := testPass.parse(strings.ToLower(dirbody))
			if err != nil {
				res.parsewarn += fmt.Sprintln("Can't parse testpass value: ", err, ". Use default noerror testpass")
				continue
			}

			res.testpass = testPass
		}
	}
}

func (p *sqlParser) loadSQLFiles(sqlpath string, ignorerrors bool) (map[string]*query, error) {
	// load queries in temp var
	qlist := make(map[string]*query)
	sqlroot := strings.TrimSuffix(sqlpath, "/")

	walkFn := func(path string, file os.FileInfo, err error) error {
		// handle error
		if err != nil {
			if ignorerrors {
				return filepath.SkipDir
			}

			return err
		}

		// skip dir and not sql files
		if file.IsDir() || strings.ToLower(filepath.Ext(path)) != ".sql" {
			return nil
		}

		// init query
		name := path[len(sqlroot):]
		name = strings.TrimSuffix(name, filepath.Ext(name))
		q := query{name: name}

		// read sql file
		if bin, errio := ioutil.ReadFile(path); errio == nil {
			p.parse(string(bin), &q)
		} else {
			if !ignorerrors {
				return errio
			}
			q.err = errio
		}
		qlist[q.name] = &q

		return nil
	}

	if err := filepath.Walk(sqlroot, walkFn); err != nil {
		return nil, err
	}

	return qlist, nil
}
