package oas

import (
	"encoding/json"
	"errors"
	"github.com/chanced/openapi"
	"github.com/google/martian/har"
	"github.com/up9inc/mizu/shared/logger"
	"strconv"
	"strings"
)

func exampleResolver(ref string) (*openapi.ExampleObj, error) {
	return nil, errors.New("JSON references are not supported at the moment: " + ref)
}

func responseResolver(ref string) (*openapi.ResponseObj, error) {
	return nil, errors.New("JSON references are not supported at the moment: " + ref)
}

func reqBodyResolver(ref string) (*openapi.RequestBodyObj, error) {
	return nil, errors.New("JSON references are not supported at the moment: " + ref)
}

func paramResolver(ref string) (*openapi.ParameterObj, error) {
	return nil, errors.New("JSON references are not supported at the moment: " + ref)
}

func headerResolver(ref string) (*openapi.HeaderObj, error) {
	return nil, errors.New("JSON references are not supported at the moment: " + ref)
}

func initParams(obj **openapi.ParameterList) {
	if *obj == nil {
		var params openapi.ParameterList
		params = make([]openapi.Parameter, 0)
		*obj = &params
	}
}

func initHeaders(respObj *openapi.ResponseObj) {
	if respObj.Headers == nil {
		var created openapi.Headers
		created = map[string]openapi.Header{}
		respObj.Headers = created
	}
}

func createSimpleParam(name string, in openapi.In, ptype openapi.SchemaType) *openapi.ParameterObj {
	if name == "" {
		panic("Cannot create parameter with empty name")
	}
	required := true // FFS! https://stackoverflow.com/questions/32364027/reference-a-boolean-for-assignment-in-a-struct/32364093
	schema := new(openapi.SchemaObj)
	schema.Type = make(openapi.Types, 0)
	schema.Type = append(schema.Type, ptype)

	style := openapi.StyleSimple
	if in == openapi.InQuery {
		style = openapi.StyleForm
	}

	newParam := openapi.ParameterObj{
		Name:     name,
		In:       in,
		Style:    string(style),
		Examples: map[string]openapi.Example{},
		Schema:   schema,
		Required: &required,
	}
	return &newParam
}

func findParamByName(params *openapi.ParameterList, in openapi.In, name string) (pathParam *openapi.ParameterObj) {
	caseInsensitive := in == openapi.InHeader
	for _, param := range *params {
		paramObj, err := param.ResolveParameter(paramResolver)
		if err != nil {
			logger.Log.Warningf("Failed to resolve reference: %s", err)
			continue
		}

		if paramObj.In != in {
			continue
		}

		if paramObj.Name == name || (caseInsensitive && strings.ToLower(paramObj.Name) == strings.ToLower(name)) {
			pathParam = paramObj
			break
		}
	}
	return pathParam
}

func findHeaderByName(headers *openapi.Headers, name string) *openapi.HeaderObj {
	for hname, param := range *headers {
		hdrObj, err := param.ResolveHeader(headerResolver)
		if err != nil {
			logger.Log.Warningf("Failed to resolve reference: %s", err)
			continue
		}

		if strings.ToLower(hname) == strings.ToLower(name) {
			return hdrObj
		}
	}
	return nil
}

type NVPair struct {
	Name  string
	Value string
}

type nvParams struct {
	In             openapi.In
	Pairs          func() []NVPair
	IsIgnored      func(name string) bool
	GeneralizeName func(name string) string
}

func qstrToNVP(list []har.QueryString) []NVPair {
	res := make([]NVPair, len(list))
	for idx, val := range list {
		res[idx] = NVPair{Name: val.Name, Value: val.Value}
	}
	return res
}

func hdrToNVP(list []har.Header) []NVPair {
	res := make([]NVPair, len(list))
	for idx, val := range list {
		res[idx] = NVPair{Name: val.Name, Value: val.Value}
	}
	return res
}

func handleNameVals(gw nvParams, params **openapi.ParameterList) {
	visited := map[string]*openapi.ParameterObj{}
	for _, pair := range gw.Pairs() {
		if gw.IsIgnored(pair.Name) {
			continue
		}

		nameGeneral := gw.GeneralizeName(pair.Name)

		initParams(params)
		param := findParamByName(*params, gw.In, pair.Name)
		if param == nil {
			param = createSimpleParam(nameGeneral, gw.In, openapi.TypeString)
			appended := append(**params, param)
			*params = &appended
		}
		exmp := &param.Examples
		err := fillParamExample(&exmp, pair.Value)
		if err != nil {
			logger.Log.Warningf("Failed to add example to a parameter: %s", err)
		}
		visited[nameGeneral] = param
	}

	// maintain "required" flag
	if *params != nil {
		for _, param := range **params {
			paramObj, err := param.ResolveParameter(paramResolver)
			if err != nil {
				logger.Log.Warningf("Failed to resolve param: %s", err)
				continue
			}
			if paramObj.In != gw.In {
				continue
			}

			_, ok := visited[strings.ToLower(paramObj.Name)]
			if !ok {
				flag := false
				paramObj.Required = &flag
			}
		}
	}
}

func createHeader(ptype openapi.SchemaType) *openapi.HeaderObj {
	required := true // FFS! https://stackoverflow.com/questions/32364027/reference-a-boolean-for-assignment-in-a-struct/32364093
	schema := new(openapi.SchemaObj)
	schema.Type = make(openapi.Types, 0)
	schema.Type = append(schema.Type, ptype)

	style := openapi.StyleSimple
	newParam := openapi.HeaderObj{
		Style:    string(style),
		Examples: map[string]openapi.Example{},
		Schema:   schema,
		Required: &required,
	}
	return &newParam
}

func fillParamExample(param **openapi.Examples, exampleValue string) error {
	if **param == nil {
		**param = map[string]openapi.Example{}
	}

	cnt := 0
	for _, example := range **param {
		cnt++
		exampleObj, err := example.ResolveExample(exampleResolver)
		if err != nil {
			continue
		}

		var value string
		err = json.Unmarshal(exampleObj.Value, &value)
		if err != nil {
			logger.Log.Warningf("Failed decoding parameter example into string: %s", err)
			continue
		}

		if value == exampleValue || cnt > 5 { // 5 examples is enough
			return nil
		}
	}

	valMsg, err := json.Marshal(exampleValue)
	if err != nil {
		return err
	}

	themap := **param
	themap["example #"+strconv.Itoa(cnt)] = &openapi.ExampleObj{Value: valMsg}

	return nil
}

func longestCommonXfix(strs [][]string, pre bool) []string { // https://github.com/jpillora/longestcommon
	empty := make([]string, 0)
	//short-circuit empty list
	if len(strs) == 0 {
		return empty
	}
	xfix := strs[0]
	//short-circuit single-element list
	if len(strs) == 1 {
		return xfix
	}
	//compare first to rest
	for _, str := range strs[1:] {
		xfixl := len(xfix)
		strl := len(str)
		//short-circuit empty strings
		if xfixl == 0 || strl == 0 {
			return empty
		}
		//maximum possible length
		maxl := xfixl
		if strl < maxl {
			maxl = strl
		}
		//compare letters
		if pre {
			//prefix, iterate left to right
			for i := 0; i < maxl; i++ {
				if xfix[i] != str[i] {
					xfix = xfix[:i]
					break
				}
			}
		} else {
			//suffix, iternate right to left
			for i := 0; i < maxl; i++ {
				xi := xfixl - i - 1
				si := strl - i - 1
				if xfix[xi] != str[si] {
					xfix = xfix[xi+1:]
					break
				}
			}
		}
	}
	return xfix
}

// returns all non-nil ops in PathObj
func getOps(pathObj *openapi.PathObj) []*openapi.Operation {
	ops := []**openapi.Operation{&pathObj.Get, &pathObj.Patch, &pathObj.Put, &pathObj.Options, &pathObj.Post, &pathObj.Trace, &pathObj.Head, &pathObj.Delete}
	res := make([]*openapi.Operation, 0)
	for _, opp := range ops {
		if *opp == nil {
			continue
		}
		res = append(res, *opp)
	}
	return res
}

// parses JSON into any possible value
func anyJSON(text string) (anyVal interface{}, isJSON bool) {
	isJSON = true
	asMap := map[string]interface{}{}
	err := json.Unmarshal([]byte(text), &asMap)
	if err == nil && asMap != nil {
		return asMap, isJSON
	}

	asArray := make([]interface{}, 0)
	err = json.Unmarshal([]byte(text), &asArray)
	if err == nil && asArray != nil {
		return asArray, isJSON
	}

	asString := ""
	sPtr := &asString
	err = json.Unmarshal([]byte(text), &sPtr)
	if err == nil && sPtr != nil {
		return asString, isJSON
	}

	asInt := 0
	intPtr := &asInt
	err = json.Unmarshal([]byte(text), &intPtr)
	if err == nil && intPtr != nil {
		return asInt, isJSON
	}

	asFloat := 0.0
	floatPtr := &asFloat
	err = json.Unmarshal([]byte(text), &floatPtr)
	if err == nil && floatPtr != nil {
		return asFloat, isJSON
	}

	asBool := false
	boolPtr := &asBool
	err = json.Unmarshal([]byte(text), &boolPtr)
	if err == nil && boolPtr != nil {
		return asBool, isJSON
	}

	if text == "null" {
		return nil, isJSON
	}

	return nil, false
}
