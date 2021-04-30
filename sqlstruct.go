package main

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Directive types
type dirParam struct {
	key   string
	value string
}

type dirParamList []dirParam

func (list dirParamList) find(key string) (bool, int) {
	for i, param := range list {
		if param.key == key {
			return true, i
		}
	}

	return false, -1
}

func (list *dirParamList) readIn(regExp *regexp.Regexp, str string, keyGrp int, valueGrp int) {
	for _, match := range regExp.FindAllStringSubmatch(str, -1) {
		key := strings.ToLower(match[keyGrp])
		value := match[valueGrp]

		if key == "" {
			continue
		}

		found, _ := list.find(key)
		if !found {
			*list = append(*list, dirParam{key, value})
		}
	}
}

func (list dirParamList) toURLValues() url.Values {
	res := url.Values{}

	for _, param := range list {
		res.Add(param.key, param.value)
	}

	return res
}

// SQL param type
type paramList []string

func (list paramList) find(name string) (bool, int) {
	for i, param := range list {
		if param == name {
			return true, i
		}
	}

	return false, -1
}

func (list paramList) prepare(urlparam url.Values, filterInParams bool) ([]interface{}, error) {
	res := make([]interface{}, 0)

	for _, name := range list {
		val, ok := urlparam[name]

		if ok {
			if filterInParams {
				delete(urlparam, name)
			}

			if val[0] != "" {
				res = append(res, val[0])
				continue
			}
		}

		res = append(res, nil)
	}

	if filterInParams && len(urlparam) > 0 {
		var pstr string
		for k := range urlparam {
			if pstr == "" {
				pstr = k
				continue
			}

			pstr += ", " + k
		}

		return nil, errors.New(fmt.Sprint("Unknow input parameters: ", pstr))
	}
	return res, nil
}

// Query type
type query struct {
	name        string            // relative file path
	body        string            // sql query
	params      paramList         // parsed params names from query
	description string            // query description
	in          dirParamList      // parsed input params description
	out         dirParamList      // parsed output params description
	testparams  dirParamList      // parsed test scenario params
	testpass    queryTestPassType // condition for a successful test scenario (see testPassValueList)
	timeout     *time.Duration    // query timeout
	loadtime    time.Time         // when was the request parsing from a file
	parsewarn   string            // parse warnings
	testreport  *queryTestReport  // autotest report
	err         error             // error duryng loading/testing query
}
